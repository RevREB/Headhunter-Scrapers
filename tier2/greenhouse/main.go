// Command greenhouse scrapes public Greenhouse job boards and posts
// contract-shaped RawPostings to Headhunter-Core's ingest endpoint.
//
// Greenhouse exposes a clean public JSON API per company board:
//
//	https://boards-api.greenhouse.io/v1/boards/{token}          -> {name}
//	https://boards-api.greenhouse.io/v1/boards/{token}/jobs      -> {jobs:[...]}
//
// No auth, no tokens spent — discovery is free.
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

// Seed boards known to use Greenhouse. Invalid tokens 404 and are skipped.
const defaultCompanies = "anthropic,gitlab,figma,brex,databricks,discord,coinbase,plaid,robinhood,samsara,hashicorp,cockroachlabs,benchling,webflow,mixpanel,ramp,airtable,gusto,rippling,vercel"

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

func main() {
	ingest := env("CORE_INGEST_URL", "http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest")
	companies := strings.Split(env("GH_COMPANIES", defaultCompanies), ",")
	var keywords []string
	for _, w := range strings.Split(os.Getenv("ROLE_KEYWORDS"), ",") {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			keywords = append(keywords, w)
		}
	}

	// contract handshake
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ats": "greenhouse", "contractVersion": contractVersion, "capabilities": []string{"http-json"},
	})

	var all []rawPosting
	for _, tok := range companies {
		if tok = strings.TrimSpace(tok); tok == "" {
			continue
		}
		var board struct {
			Name string `json:"name"`
		}
		if err := getJSON("https://boards-api.greenhouse.io/v1/boards/"+tok, &board); err != nil {
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
		if err := getJSON("https://boards-api.greenhouse.io/v1/boards/"+tok+"/jobs", &jr); err != nil {
			fmt.Fprintf(os.Stderr, "[greenhouse] %s: jobs skipped (%v)\n", tok, err)
			continue
		}
		n := 0
		for _, j := range jr.Jobs {
			if len(keywords) > 0 && !matchAny(strings.ToLower(j.Title), keywords) {
				continue
			}
			raw, _ := json.Marshal(j)
			all = append(all, rawPosting{
				URL: j.AbsoluteURL, Title: j.Title, Company: company,
				Location: j.Location.Name, PostedAt: j.UpdatedAt, Raw: raw,
			})
			n++
		}
		fmt.Fprintf(os.Stderr, "[greenhouse] %s (%s): %d/%d matched\n", tok, company, n, len(jr.Jobs))
	}
	fmt.Fprintf(os.Stderr, "[greenhouse] total matched: %d\n", len(all))
	if len(all) == 0 {
		return
	}

	body, _ := json.Marshal(all)
	resp, err := httpc.Post(ingest, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[greenhouse] ingest error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Fprintf(os.Stderr, "[greenhouse] ingest -> %d %s\n", resp.StatusCode, strings.TrimSpace(string(out)))
	if resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
