// Command greenhouse scrapes public Greenhouse job boards (boards-api.greenhouse.io).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/RevREB/Headhunter-Scrapers/scraperkit"
)

const defaultCompanies = "anthropic,gitlab,figma,brex,databricks,discord,coinbase,plaid,robinhood,samsara,hashicorp,cockroachlabs,benchling,webflow,mixpanel,ramp,airtable,gusto,rippling,vercel"

func main() { scraperkit.Main("greenhouse", fetch) }

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	for _, tok := range strings.Split(scraperkit.Env("GH_COMPANIES", defaultCompanies), ",") {
		if tok = strings.TrimSpace(tok); tok == "" {
			continue
		}
		var board struct {
			Name string `json:"name"`
		}
		if err := scraperkit.GetJSON("https://boards-api.greenhouse.io/v1/boards/"+tok, nil, &board); err != nil {
			fmt.Fprintf(os.Stderr, "[greenhouse] %s: board skipped (%v)\n", tok, err)
			continue
		}
		company := board.Name
		if company == "" {
			company = tok
		}
		var jr struct {
			Jobs []struct {
				Title       string `json:"title"`
				AbsoluteURL string `json:"absolute_url"`
				UpdatedAt   string `json:"updated_at"`
				Location    struct {
					Name string `json:"name"`
				} `json:"location"`
			} `json:"jobs"`
		}
		if err := scraperkit.GetJSON("https://boards-api.greenhouse.io/v1/boards/"+tok+"/jobs", nil, &jr); err != nil {
			fmt.Fprintf(os.Stderr, "[greenhouse] %s: jobs skipped (%v)\n", tok, err)
			continue
		}
		n := 0
		for _, j := range jr.Jobs {
			raw, _ := json.Marshal(j)
			if emit(scraperkit.RawPosting{URL: j.AbsoluteURL, Title: j.Title, Company: company, Location: j.Location.Name, PostedAt: j.UpdatedAt, Raw: raw}) {
				n++
			}
		}
		fmt.Fprintf(os.Stderr, "[greenhouse] %s (%s): %d/%d matched\n", tok, company, n, len(jr.Jobs))
	}
	return nil
}
