// Command amazon scrapes Amazon's global job board and posts contract-shaped
// RawPostings to Headhunter-Core's ingest endpoint.
//
// Amazon is not on a standard ATS: it runs one giant global board at
// amazon.jobs with a public search.json endpoint:
//
//	https://www.amazon.jobs/en/search.json?base_query={q}&loc_query=&sort=recent&
//	  result_limit=100&offset={offset}&normalized_country_code[]={country}
//	  -> {hits:<int>, jobs:[{title, job_path, location, posted_date}, ...]}
//
// result_limit is capped at 100 by the API, so we page via offset. No auth,
// no tokens spent.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const atsName = "amazon"

// maxPages bounds pagination per query: 100 results/page * 5 = at most 500.
const maxPages = 5

// resultLimit is the API-capped page size.
const resultLimit = 100

// defaultQueries targets the infra/platform/SRE role shapes.
const defaultQueries = "site reliability engineer,infrastructure engineer,platform engineer,devops engineer,cloud infrastructure,kubernetes engineer"

// rawPosting is the contract-shaped output, identical across all scrapers.
type rawPosting struct {
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Company  string          `json:"company"`
	Location string          `json:"location,omitempty"`
	Comp     string          `json:"comp,omitempty"`
	PostedAt string          `json:"postedAt,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// amazonResponse is the shape of the amazon.jobs search.json response.
type amazonResponse struct {
	Hits int               `json:"hits"`
	Jobs []json.RawMessage `json:"jobs"`
}

// amazonJob is one job entry within the response.
type amazonJob struct {
	Title      string `json:"title"`
	JobPath    string `json:"job_path"`
	Location   string `json:"location"`
	PostedDate string `json:"posted_date"`
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// matchAny reports whether the lowercased title contains any of the keywords.
// An empty keyword slice matches everything (no filtering).
func matchAny(title string, kws []string) bool {
	if len(kws) == 0 {
		return true
	}
	lt := strings.ToLower(title)
	for _, k := range kws {
		if k != "" && strings.Contains(lt, k) {
			return true
		}
	}
	return false
}

// splitQueries splits a comma list into trimmed, non-empty query terms.
func splitQueries(s string) []string {
	var out []string
	for _, q := range strings.Split(s, ",") {
		if q = strings.TrimSpace(q); q != "" {
			out = append(out, q)
		}
	}
	return out
}

// parseKeywords splits a comma list into lowercased, trimmed keywords.
func parseKeywords(s string) []string {
	var out []string
	for _, k := range strings.Split(s, ",") {
		if k = strings.ToLower(strings.TrimSpace(k)); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// searchURL builds one paginated amazon.jobs search.json URL.
func searchURL(query, country string, offset int) string {
	v := url.Values{}
	v.Set("base_query", query)
	v.Set("loc_query", "")
	v.Set("sort", "recent")
	v.Set("result_limit", fmt.Sprintf("%d", resultLimit))
	v.Set("offset", fmt.Sprintf("%d", offset))
	v.Set("normalized_country_code[]", country)
	return "https://www.amazon.jobs/en/search.json?" + v.Encode()
}

// parsePostedDate turns an amazon.jobs posted_date like "July  3, 2026"
// (note the double space) into an RFC3339 timestamp. On any parse failure it
// returns "" so the field is simply omitted.
func parsePostedDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Collapse runs of whitespace to a single space.
	s = strings.Join(strings.Fields(s), " ")
	t, err := time.Parse("January 2, 2006", s)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// fetchPage GETs one page of the amazon.jobs search endpoint and decodes it.
func fetchPage(httpc *http.Client, query, country string, offset int) (*amazonResponse, error) {
	req, err := http.NewRequest(http.MethodGet, searchURL(query, country, offset), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; HeadhunterBot/1.0)")
	req.Header.Set("Accept", "application/json")

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var ar amazonResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return nil, err
	}
	return &ar, nil
}

// scrapeQuery pages through one query term and returns its matched postings,
// the total jobs seen, and any error. Dedup happens in the caller via seen.
func scrapeQuery(httpc *http.Client, query, country string, kws []string, seen map[string]bool) ([]rawPosting, int, error) {
	var out []rawPosting
	total := 0
	for page := 0; page < maxPages; page++ {
		offset := page * resultLimit
		ar, err := fetchPage(httpc, query, country, offset)
		if err != nil {
			return nil, 0, err
		}
		if len(ar.Jobs) == 0 {
			break
		}
		for _, raw := range ar.Jobs {
			total++
			var j amazonJob
			if err := json.Unmarshal(raw, &j); err != nil {
				continue
			}
			if !matchAny(j.Title, kws) {
				continue
			}
			u := "https://www.amazon.jobs" + j.JobPath
			if seen[u] {
				continue
			}
			seen[u] = true
			out = append(out, rawPosting{
				URL:      u,
				Title:    j.Title,
				Company:  "Amazon",
				Location: j.Location,
				PostedAt: parsePostedDate(j.PostedDate),
				Raw:      raw,
			})
		}
		if offset+resultLimit >= ar.Hits {
			break
		}
	}
	return out, total, nil
}

func main() {
	// Handshake first.
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ats":             atsName,
		"contractVersion": "1.0.0",
		"capabilities":    []string{"http-json"},
	})

	ingestURL := env("CORE_INGEST_URL", "http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest")
	kws := parseKeywords(env("ROLE_KEYWORDS", ""))
	queries := splitQueries(env("AMAZON_QUERIES", defaultQueries))
	country := env("AMAZON_COUNTRY", "USA")

	httpc := &http.Client{Timeout: 30 * time.Second}
	seen := map[string]bool{}

	var all []rawPosting
	for _, q := range queries {
		posts, total, err := scrapeQuery(httpc, q, country, kws, seen)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] %s: skipped (%v)\n", atsName, q, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "[%s] %s: %d/%d\n", atsName, q, len(posts), total)
		all = append(all, posts...)
	}
	fmt.Fprintf(os.Stderr, "[%s] total matched: %d\n", atsName, len(all))

	if len(all) == 0 {
		return
	}

	payload, err := json.Marshal(all)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] marshal error: %v\n", atsName, err)
		os.Exit(1)
	}
	resp, err := httpc.Post(ingestURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] ingest error: %v\n", atsName, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	fmt.Printf("[%s] ingest -> %d %s\n", atsName, resp.StatusCode, strings.TrimSpace(string(body)))
	if resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
