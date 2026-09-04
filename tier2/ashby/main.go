// Command ashby scrapes public Ashby job boards and posts
// contract-shaped RawPostings to Headhunter-Core's ingest endpoint.
//
// Ashby exposes a public posting API per job board:
//
//	https://api.ashbyhq.com/posting-api/job-board/{board}?includeCompensation=true
//	  -> {jobs:[{title, location, jobUrl, employmentType, publishedAt, isListed}, ...]}
//
// No auth, no tokens spent. A 404/non-200 means the board name is wrong — skip it.
// The posting-api carries no org name, so Company is derived from the board slug.
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

const contractVersion = "1.0.0"

// Seed boards known to use Ashby. Invalid slugs 404 and are skipped.
const defaultCompanies = "openai,ramp,linear,notion,mistral,cohere,runway,perplexity-ai,together-ai,modal,replicate,character"

type rawPosting struct {
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Company  string          `json:"company"`
	Location string          `json:"location,omitempty"`
	Comp     string          `json:"comp,omitempty"`
	PostedAt string          `json:"postedAt,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var httpc = &http.Client{Timeout: 30 * time.Second}

func getJSON(url string, v any) error {
	resp, err := httpc.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func matchAny(s string, kws []string) bool {
	for _, k := range kws {
		if k != "" && strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// titleCaseBoard turns an Ashby board slug into a display company name:
// dashes become spaces and each word is capitalized, e.g.
// "perplexity-ai" -> "Perplexity Ai", "together-ai" -> "Together Ai".
func titleCaseBoard(board string) string {
	parts := strings.FieldsFunc(board, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})
	for i, p := range parts {
		if p == "" {
			continue
		}
		r := []rune(strings.ToLower(p))
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		parts[i] = string(r)
	}
	return strings.Join(parts, " ")
}

// firstNonEmpty returns the first non-empty string, used to pick a posted date
// from whichever of several date fields Ashby happens to populate.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// jobURL prefers the API-supplied apply URL, falling back to a constructed
// public URL when only an id is present.
func jobURL(board, jobURLField, id string) string {
	if jobURLField != "" {
		return jobURLField
	}
	if id != "" {
		return "https://jobs.ashbyhq.com/" + board + "/" + id
	}
	return ""
}

func main() {
	ingest := env("CORE_INGEST_URL", "http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest")
	companies := strings.Split(env("ASHBY_COMPANIES", defaultCompanies), ",")
	var keywords []string
	for _, w := range strings.Split(os.Getenv("ROLE_KEYWORDS"), ",") {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			keywords = append(keywords, w)
		}
	}

	// contract handshake
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ats": "ashby", "contractVersion": contractVersion, "capabilities": []string{"http-json"},
	})

	var all []rawPosting
	for _, board := range companies {
		if board = strings.TrimSpace(board); board == "" {
			continue
		}
		company := titleCaseBoard(board)
		var jr struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		url := "https://api.ashbyhq.com/posting-api/job-board/" + board + "?includeCompensation=true"
		if err := getJSON(url, &jr); err != nil {
			fmt.Fprintf(os.Stderr, "[ashby] %s: skipped (%v)\n", board, err)
			continue
		}
		n := 0
		for _, rawJob := range jr.Jobs {
			var j struct {
				ID            string `json:"id"`
				Title         string `json:"title"`
				Location      string `json:"location"`
				JobURL        string `json:"jobUrl"`
				EmploymentT   string `json:"employmentType"`
				PublishedAt   string `json:"publishedAt"`
				PublishedDate string `json:"publishedDate"`
				UpdatedAt     string `json:"updatedAt"`
				IsListed      *bool  `json:"isListed"`
			}
			if err := json.Unmarshal(rawJob, &j); err != nil {
				continue
			}
			if j.IsListed != nil && !*j.IsListed {
				continue
			}
			if len(keywords) > 0 && !matchAny(strings.ToLower(j.Title), keywords) {
				continue
			}
			all = append(all, rawPosting{
				URL:      jobURL(board, j.JobURL, j.ID),
				Title:    j.Title,
				Company:  company,
				Location: j.Location,
				PostedAt: firstNonEmpty(j.PublishedAt, j.PublishedDate, j.UpdatedAt),
				Raw:      rawJob,
			})
			n++
		}
		fmt.Fprintf(os.Stderr, "[ashby] %s (%s): %d/%d matched\n", board, company, n, len(jr.Jobs))
	}
	fmt.Fprintf(os.Stderr, "[ashby] total matched: %d\n", len(all))
	if len(all) == 0 {
		return
	}

	body, _ := json.Marshal(all)
	resp, err := httpc.Post(ingest, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ashby] ingest error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Fprintf(os.Stderr, "[ashby] ingest -> %d %s\n", resp.StatusCode, strings.TrimSpace(string(out)))
	if resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
