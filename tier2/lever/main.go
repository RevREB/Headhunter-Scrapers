// Command lever scrapes public Lever job boards and posts
// contract-shaped RawPostings to Headhunter-Core's ingest endpoint.
//
// Lever exposes a clean public JSON API per company handle:
//
//	https://api.lever.co/v0/postings/{handle}?mode=json  -> [ {posting}, ... ]
//
// The response is a bare JSON array (not wrapped). No auth, no tokens spent —
// discovery is free. A 404 means the handle is not on Lever and is skipped.
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

// Seed handles known to use Lever. Invalid handles 404 and are skipped.
const defaultCompanies = "plaid,brex,ramp,notion,mistral,cohere,scale,netflix,spotify,attentive,ironclad,benchling,gopuff,checkr"

type rawPosting struct {
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Company  string          `json:"company"`
	Location string          `json:"location,omitempty"`
	Comp     string          `json:"comp,omitempty"`
	PostedAt string          `json:"postedAt,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// leverPosting is the subset of a Lever posting we map into the contract.
type leverPosting struct {
	Text       string `json:"text"`
	HostedURL  string `json:"hostedUrl"`
	Categories struct {
		Location   string `json:"location"`
		Team       string `json:"team"`
		Commitment string `json:"commitment"`
	} `json:"categories"`
	CreatedAt int64 `json:"createdAt"`
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var httpc = &http.Client{Timeout: 30 * time.Second}

func matchAny(s string, kws []string) bool {
	for _, k := range kws {
		if k != "" && strings.Contains(s, k) {
			return true
		}
	}
	return false
}

// postingsURL builds the Lever public postings endpoint for a handle.
func postingsURL(handle string) string {
	return "https://api.lever.co/v0/postings/" + handle + "?mode=json"
}

// companyName title-cases a Lever handle for display, since Lever postings
// carry no company name. e.g. "plaid" -> "Plaid".
func companyName(handle string) string {
	if handle == "" {
		return handle
	}
	return strings.ToUpper(handle[:1]) + handle[1:]
}

// postedAt converts a millisecond epoch timestamp to RFC3339, guarding 0.
func postedAt(ms int64) string {
	if ms == 0 {
		return ""
	}
	return time.UnixMilli(ms).UTC().Format(time.RFC3339)
}

func main() {
	ingest := env("CORE_INGEST_URL", "http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest")
	companies := strings.Split(env("LEVER_COMPANIES", defaultCompanies), ",")
	var keywords []string
	for _, w := range strings.Split(os.Getenv("ROLE_KEYWORDS"), ",") {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			keywords = append(keywords, w)
		}
	}

	// contract handshake
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ats": "lever", "contractVersion": contractVersion, "capabilities": []string{"http-json"},
	})

	var all []rawPosting
	for _, handle := range companies {
		if handle = strings.TrimSpace(handle); handle == "" {
			continue
		}

		resp, err := httpc.Get(postingsURL(handle))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[lever] %s: skipped (%v)\n", handle, err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			fmt.Fprintf(os.Stderr, "[lever] %s: skipped (status %d)\n", handle, resp.StatusCode)
			continue
		}

		// Decode the bare array into raw messages so the original posting JSON
		// is preserved verbatim for Raw; the typed view is only for mapping.
		var items []json.RawMessage
		if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
			resp.Body.Close()
			fmt.Fprintf(os.Stderr, "[lever] %s: skipped (%v)\n", handle, err)
			continue
		}
		resp.Body.Close()

		company := companyName(handle)
		n := 0
		for _, item := range items {
			var p leverPosting
			if err := json.Unmarshal(item, &p); err != nil {
				continue
			}
			if len(keywords) > 0 && !matchAny(strings.ToLower(p.Text), keywords) {
				continue
			}
			all = append(all, rawPosting{
				URL: p.HostedURL, Title: p.Text, Company: company,
				Location: p.Categories.Location, PostedAt: postedAt(p.CreatedAt), Raw: item,
			})
			n++
		}
		fmt.Fprintf(os.Stderr, "[lever] %s (%s): %d/%d matched\n", handle, company, n, len(items))
	}
	fmt.Fprintf(os.Stderr, "[lever] total matched: %d\n", len(all))
	if len(all) == 0 {
		return
	}

	body, _ := json.Marshal(all)
	resp, err := httpc.Post(ingest, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[lever] ingest error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	fmt.Fprintf(os.Stderr, "[lever] ingest -> %d %s\n", resp.StatusCode, strings.TrimSpace(string(out)))
	if resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
