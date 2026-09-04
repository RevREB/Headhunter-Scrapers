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
		// ?content=true returns each job's full HTML JD in the same list call, so
		// we keep the whole job object (content + departments + offices) as Raw.
		var jr struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if err := scraperkit.GetJSON("https://boards-api.greenhouse.io/v1/boards/"+tok+"/jobs?content=true", nil, &jr); err != nil {
			fmt.Fprintf(os.Stderr, "[greenhouse] %s: jobs skipped (%v)\n", tok, err)
			continue
		}
		n := 0
		for _, rawJob := range jr.Jobs {
			var j struct {
				Title          string `json:"title"`
				AbsoluteURL    string `json:"absolute_url"`
				UpdatedAt      string `json:"updated_at"`
				FirstPublished string `json:"first_published"`
				Location       struct {
					Name string `json:"name"`
				} `json:"location"`
			}
			if json.Unmarshal(rawJob, &j) != nil {
				continue
			}
			posted := j.FirstPublished
			if posted == "" {
				posted = j.UpdatedAt
			}
			if emit(scraperkit.RawPosting{URL: j.AbsoluteURL, Title: j.Title, Company: company, Location: j.Location.Name, PostedAt: posted, Raw: rawJob}) {
				n++
			}
		}
		fmt.Fprintf(os.Stderr, "[greenhouse] %s (%s): %d/%d matched\n", tok, company, n, len(jr.Jobs))
	}
	return nil
}
