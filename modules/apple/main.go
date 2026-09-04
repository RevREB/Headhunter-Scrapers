// Command apple scrapes jobs.apple.com. DEFERRED: the API is CSRF/session-gated
// (HTTP 436 without a token) and likely datacenter-IP-blocked; kept in-repo and
// written defensively (any error/shape is logged + skipped, never fatal) so it
// can be revived once a token/session flow is added.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/RevREB/Headhunter-Scrapers/scraperkit"
)

const (
	searchEndpoint = "https://jobs.apple.com/api/v1/search"
	userAgent      = "Mozilla/5.0 (compatible; HeadhunterBot/1.0)"
	defaultQueries = "infrastructure,platform,site reliability,devops,cloud,kubernetes"
)

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
		SearchResults []appleSearchResult `json:"searchResults"`
	} `json:"res"`
}

func first(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// detailsURL builds the canonical posting URL; slug optional.
func detailsURL(id, slug string) string {
	if id = strings.TrimSpace(id); id == "" {
		return ""
	}
	u := "https://jobs.apple.com/en-us/details/" + id
	if slug = strings.TrimSpace(slug); slug != "" {
		u += "/" + slug
	}
	return u
}

// parseLocations tolerates []{name} or a bare string.
func parseLocations(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var objs []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &objs) == nil && len(objs) > 0 {
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
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	return ""
}

func fetchPage(q string, page int) ([]appleSearchResult, error) {
	body, _ := json.Marshal(map[string]any{
		"query": q, "page": page, "locale": "en-us", "sort": "newest",
		"filters": map[string]any{"postingpostLocation": []string{"postLocation-USA"}},
	})
	req, err := http.NewRequest(http.MethodPost, searchEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://jobs.apple.com/en-us/search")
	req.Header.Set("User-Agent", userAgent)
	resp, err := scraperkit.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode != http.StatusOK {
		snip := string(data)
		if len(snip) > 160 {
			snip = snip[:160]
		}
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, snip)
	}
	var ar appleResponse
	if err := json.Unmarshal(data, &ar); err != nil {
		return nil, err
	}
	return ar.Res.SearchResults, nil
}

func main() { scraperkit.Main("apple", fetch) }

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	for _, q := range strings.Split(scraperkit.Env("APPLE_QUERIES", defaultQueries), ",") {
		if q = strings.TrimSpace(q); q == "" {
			continue
		}
		matched, total := 0, 0
		var perr error
		for page := 1; page <= 8; page++ {
			results, err := fetchPage(q, page)
			if err != nil {
				perr = err
				break
			}
			if len(results) == 0 {
				break
			}
			for _, r := range results {
				total++
				if emit(scraperkit.RawPosting{
					URL:      detailsURL(first(r.PositionID, r.ID), r.TransformedPostingTitle),
					Title:    first(r.PostingTitle, r.PositionTitle, r.Title),
					Company:  "Apple",
					Location: parseLocations(r.Locations),
					PostedAt: first(r.PostDateInGMT, r.PostingDate),
					Raw:      mustRaw(r),
				}) {
					matched++
				}
			}
		}
		if perr != nil {
			fmt.Fprintf(os.Stderr, "[apple] %s: skipped (%v)\n", q, perr)
			continue
		}
		fmt.Fprintf(os.Stderr, "[apple] %s: %d/%d\n", q, matched, total)
	}
	return nil
}

func mustRaw(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
