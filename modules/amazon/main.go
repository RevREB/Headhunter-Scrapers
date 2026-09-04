// Command amazon scrapes Amazon's global board (amazon.jobs/search.json) with a
// set of base_query terms (+ country facet), paginating each.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/RevREB/Headhunter-Scrapers/scraperkit"
)

const (
	maxPages       = 5   // 100/page * 5 = up to 500 per query
	resultLimit    = 100 // API cap
	defaultQueries = "site reliability engineer,infrastructure engineer,platform engineer,devops engineer,cloud infrastructure,kubernetes engineer"
)

type amazonResponse struct {
	Hits int               `json:"hits"`
	Jobs []json.RawMessage `json:"jobs"`
}

func splitList(s string) []string {
	var out []string
	for _, q := range strings.Split(s, ",") {
		if q = strings.TrimSpace(q); q != "" {
			out = append(out, q)
		}
	}
	return out
}

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

// parsePostedDate turns "July  3, 2026" (padded day) into RFC3339, else "".
func parsePostedDate(s string) string {
	if s = strings.Join(strings.Fields(s), " "); s == "" {
		return ""
	}
	t, err := time.Parse("January 2, 2006", s)
	if err != nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

var hdrs = map[string]string{"User-Agent": "Mozilla/5.0 (compatible; HeadhunterBot/1.0)", "Accept": "application/json"}

func main() { scraperkit.Main("amazon", fetch) }

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	country := scraperkit.Env("AMAZON_COUNTRY", "USA")
	for _, q := range splitList(scraperkit.Env("AMAZON_QUERIES", defaultQueries)) {
		matched, total := 0, 0
		var perr error
		for page := 0; page < maxPages; page++ {
			offset := page * resultLimit
			var ar amazonResponse
			if err := scraperkit.GetJSON(searchURL(q, country, offset), hdrs, &ar); err != nil {
				perr = err
				break
			}
			if len(ar.Jobs) == 0 {
				break
			}
			for _, raw := range ar.Jobs {
				total++
				var j struct {
					Title      string `json:"title"`
					JobPath    string `json:"job_path"`
					Location   string `json:"location"`
					PostedDate string `json:"posted_date"`
				}
				if json.Unmarshal(raw, &j) != nil {
					continue
				}
				if emit(scraperkit.RawPosting{URL: "https://www.amazon.jobs" + j.JobPath, Title: j.Title, Company: "Amazon", Location: j.Location, PostedAt: parsePostedDate(j.PostedDate), Raw: raw}) {
					matched++
				}
			}
			if offset+resultLimit >= ar.Hits {
				break
			}
		}
		if perr != nil {
			fmt.Fprintf(os.Stderr, "[amazon] %s: skipped (%v)\n", q, perr)
			continue
		}
		fmt.Fprintf(os.Stderr, "[amazon] %s: %d/%d\n", q, matched, total)
	}
	return nil
}
