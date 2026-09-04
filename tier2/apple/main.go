// Command apple is a Tier-2 custom-company scraper for Headhunter.
//
// It queries Apple's public jobs search API (jobs.apple.com), maps the
// results into Headhunter's rawPosting shape, and POSTs them to the core
// ingest endpoint. Apple's API shape is not officially documented, so every
// per-query fetch is defensive: any HTTP/decoding error or unexpected shape is
// logged and skipped rather than fatal, so the endpoint can be adjusted after
// a live test.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const atsName = "apple"

// rawPosting is Headhunter's canonical ingest shape (identical across scrapers).
type rawPosting struct {
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Company  string          `json:"company"`
	Location string          `json:"location,omitempty"`
	Comp     string          `json:"comp,omitempty"`
	PostedAt string          `json:"postedAt,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// env returns the value of environment variable k, or def when unset/empty.
func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// splitQueries splits a comma-separated list into trimmed, non-empty terms.
func splitQueries(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if q := strings.TrimSpace(part); q != "" {
			out = append(out, q)
		}
	}
	return out
}

// splitKeywords splits a comma-separated list into trimmed, lowercased,
// non-empty keywords.
func splitKeywords(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if k := strings.TrimSpace(strings.ToLower(part)); k != "" {
			out = append(out, k)
		}
	}
	return out
}

// matchesKeywords reports whether the lowercased title contains any keyword.
// An empty keyword set matches everything.
func matchesKeywords(title string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	lt := strings.ToLower(title)
	for _, k := range keywords {
		if strings.Contains(lt, k) {
			return true
		}
	}
	return false
}

// detailsURL builds the canonical jobs.apple.com posting URL. The slug is
// optional; when absent the trailing segment is omitted.
func detailsURL(id, slug string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	base := "https://jobs.apple.com/en-us/details/" + id
	if slug = strings.TrimSpace(slug); slug != "" {
		base += "/" + slug
	}
	return base
}

// appleSearchResult tolerates the several field-name variants Apple's API has
// used. Missing fields decode to their zero values.
type appleSearchResult struct {
	ID                      string          `json:"id"`
	PositionID              string          `json:"positionId"`
	PostingTitle            string          `json:"postingTitle"`
	PositionTitle           string          `json:"positionTitle"`
	Title                   string          `json:"title"`
	TransformedPostingTitle string          `json:"transformedPostingTitle"`
	PostDateInGMT           string          `json:"postDateInGMT"`
	PostingDate             string          `json:"postingDate"`
	Locations               json.RawMessage `json:"locations"`
}

type appleResponse struct {
	Res struct {
		TotalRecords  int                 `json:"totalRecords"`
		SearchResults []appleSearchResult `json:"searchResults"`
	} `json:"res"`
}

// pickTitle returns the first non-empty title variant.
func pickTitle(r appleSearchResult) string {
	for _, t := range []string{r.PostingTitle, r.PositionTitle, r.Title} {
		if s := strings.TrimSpace(t); s != "" {
			return s
		}
	}
	return ""
}

// pickID returns the first non-empty id variant (positionId preferred).
func pickID(r appleSearchResult) string {
	for _, id := range []string{r.PositionID, r.ID} {
		if s := strings.TrimSpace(id); s != "" {
			return s
		}
	}
	return ""
}

// pickDate returns the first non-empty date variant.
func pickDate(r appleSearchResult) string {
	for _, d := range []string{r.PostDateInGMT, r.PostingDate} {
		if s := strings.TrimSpace(d); s != "" {
			return s
		}
	}
	return ""
}

// parseLocations tolerates locations being either an array of {name} objects
// or a bare string, and joins the names with ", ".
func parseLocations(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try []{name}.
	var objs []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &objs); err == nil && len(objs) > 0 {
		var names []string
		for _, o := range objs {
			if n := strings.TrimSpace(o.Name); n != "" {
				names = append(names, n)
			}
		}
		if len(names) > 0 {
			return strings.Join(names, ", ")
		}
	}
	// Try a bare string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

// searchRequestBody builds the POST body for a query and page.
func searchRequestBody(q string, page int) ([]byte, error) {
	body := map[string]any{
		"query":  q,
		"page":   page,
		"locale": "en-us",
		"sort":   "newest",
		"filters": map[string]any{
			"postingpostLocation": []string{"postLocation-USA"},
		},
	}
	return json.Marshal(body)
}

const searchEndpoint = "https://jobs.apple.com/api/v1/search"

const userAgent = "Mozilla/5.0 (compatible; HeadhunterBot/1.0)"

// fetchPage fetches a single page of results for a query.
func fetchPage(httpc *http.Client, q string, page int) ([]appleSearchResult, error) {
	body, err := searchRequestBody(q, page)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, searchEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://jobs.apple.com/en-us/search")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(data)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, snippet)
	}
	var ar appleResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return ar.Res.SearchResults, nil
}

func main() {
	// Handshake first.
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ats":             atsName,
		"contractVersion": "1.0.0",
		"capabilities":    []string{"http-json"},
	})

	coreURL := env("CORE_INGEST_URL", "http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest")
	keywords := splitKeywords(env("ROLE_KEYWORDS", ""))
	queries := splitQueries(env("APPLE_QUERIES", "infrastructure,platform,site reliability,devops,cloud,kubernetes"))

	httpc := &http.Client{Timeout: 30 * time.Second}

	seen := map[string]bool{}
	var postings []rawPosting

	for _, q := range queries {
		matched, total := 0, 0
		var qErr error

		for page := 1; page <= 8; page++ {
			results, err := fetchPage(httpc, q, page)
			if err != nil {
				qErr = err
				break
			}
			if len(results) == 0 {
				break
			}
			for _, r := range results {
				total++
				title := pickTitle(r)
				if title == "" {
					continue
				}
				if !matchesKeywords(title, keywords) {
					continue
				}
				id := pickID(r)
				url := detailsURL(id, r.TransformedPostingTitle)
				if url == "" {
					continue
				}
				if seen[url] {
					continue
				}
				seen[url] = true

				raw, _ := json.Marshal(r)
				postings = append(postings, rawPosting{
					URL:      url,
					Title:    title,
					Company:  "Apple",
					Location: parseLocations(r.Locations),
					PostedAt: pickDate(r),
					Raw:      raw,
				})
				matched++
			}
		}

		if qErr != nil {
			fmt.Fprintf(os.Stderr, "[%s] %s: skipped (%v)\n", atsName, q, qErr)
			continue
		}
		fmt.Fprintf(os.Stderr, "[%s] %s: %d/%d\n", atsName, q, matched, total)
	}

	fmt.Fprintf(os.Stderr, "[%s] total matched: %d\n", atsName, len(postings))

	if len(postings) == 0 {
		return
	}

	payload, err := json.Marshal(postings)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] marshal error: %v\n", atsName, err)
		os.Exit(1)
	}

	req, err := http.NewRequest(http.MethodPost, coreURL, bytes.NewReader(payload))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] ingest request error: %v\n", atsName, err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpc.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] ingest error: %v\n", atsName, err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("[%s] ingest -> %d %s\n", atsName, resp.StatusCode, strings.TrimSpace(string(respBody)))
	if resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
