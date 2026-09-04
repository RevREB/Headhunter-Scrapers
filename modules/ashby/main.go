// Command ashby scrapes public Ashby job boards (api.ashbyhq.com posting-api).
// The posting-api carries no org name, so Company is derived from the board slug.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/RevREB/Headhunter-Scrapers/scraperkit"
)

const defaultCompanies = "openai,ramp,linear,notion,mistral,cohere,runway,perplexity-ai,together-ai,modal,replicate,character"

// titleCaseBoard turns a board slug into a display name: "perplexity-ai" -> "Perplexity Ai".
func titleCaseBoard(board string) string {
	parts := strings.FieldsFunc(board, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func jobURL(board, jobURLField, id string) string {
	if jobURLField != "" {
		return jobURLField
	}
	if id != "" {
		return "https://jobs.ashbyhq.com/" + board + "/" + id
	}
	return ""
}

func main() { scraperkit.Main("ashby", fetch) }

func fetch(cfg scraperkit.Config, emit func(scraperkit.RawPosting) bool) error {
	for _, board := range strings.Split(scraperkit.Env("ASHBY_COMPANIES", defaultCompanies), ",") {
		if board = strings.TrimSpace(board); board == "" {
			continue
		}
		company := titleCaseBoard(board)
		var jr struct {
			Jobs []json.RawMessage `json:"jobs"`
		}
		if err := scraperkit.GetJSON("https://api.ashbyhq.com/posting-api/job-board/"+board+"?includeCompensation=true", nil, &jr); err != nil {
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
				PublishedAt   string `json:"publishedAt"`
				PublishedDate string `json:"publishedDate"`
				UpdatedAt     string `json:"updatedAt"`
				IsListed      *bool  `json:"isListed"`
				Compensation  struct {
					CompensationTierSummary string `json:"compensationTierSummary"`
				} `json:"compensation"`
			}
			if json.Unmarshal(rawJob, &j) != nil {
				continue
			}
			if j.IsListed != nil && !*j.IsListed {
				continue
			}
			if emit(scraperkit.RawPosting{
				URL: jobURL(board, j.JobURL, j.ID), Title: j.Title, Company: company,
				Location: j.Location, Comp: j.Compensation.CompensationTierSummary,
				PostedAt: firstNonEmpty(j.PublishedAt, j.PublishedDate, j.UpdatedAt), Raw: rawJob,
			}) {
				n++
			}
		}
		fmt.Fprintf(os.Stderr, "[ashby] %s (%s): %d/%d matched\n", board, company, n, len(jr.Jobs))
	}
	return nil
}
