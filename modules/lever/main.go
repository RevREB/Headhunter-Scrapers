// Command lever scrapes public Lever job boards (api.lever.co). The postings
// endpoint returns a bare JSON array; a 404 means the handle isn't on Lever.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/RevREB/Headhunter-Scrapers/scraperkit"
)

const defaultCompanies = "plaid,brex,ramp,notion,mistral,cohere,scale,netflix,spotify,attentive,ironclad,benchling,gopuff,checkr"

type leverPosting struct {
	Text       string `json:"text"`
	HostedURL  string `json:"hostedUrl"`
	Categories struct {
		Location string `json:"location"`
	} `json:"categories"`
	CreatedAt int64 `json:"createdAt"`
}

func postingsURL(handle string) string {
	return "https://api.lever.co/v0/postings/" + handle + "?mode=json"
}

// companyName title-cases a Lever handle (postings carry no company name).
func companyName(handle string) string {
	if handle == "" {
		return handle
	}
	return strings.ToUpper(handle[:1]) + handle[1:]
}

// postedAt converts a millisecond epoch to RFC3339, guarding 0.
func postedAt(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func main() { scraperkit.Main("lever", fetch) }

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	for _, handle := range strings.Split(scraperkit.Env("LEVER_COMPANIES", defaultCompanies), ",") {
		if handle = strings.TrimSpace(handle); handle == "" {
			continue
		}
		var items []json.RawMessage
		if err := scraperkit.GetJSON(postingsURL(handle), nil, &items); err != nil {
			fmt.Fprintf(os.Stderr, "[lever] %s: skipped (%v)\n", handle, err)
			continue
		}
		company := companyName(handle)
		n := 0
		for _, item := range items {
			var p leverPosting
			if json.Unmarshal(item, &p) != nil {
				continue
			}
			if emit(scraperkit.RawPosting{URL: p.HostedURL, Title: p.Text, Company: company, Location: p.Categories.Location, PostedAt: postedAt(p.CreatedAt), Raw: item}) {
				n++
			}
		}
		fmt.Fprintf(os.Stderr, "[lever] %s (%s): %d/%d matched\n", handle, company, n, len(items))
	}
	return nil
}
