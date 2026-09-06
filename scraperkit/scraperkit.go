// Package scraperkit is the shared plumbing every Headhunter scraper module
// repeats: env config, the contract handshake, keyword filtering, URL dedup, and
// POSTing RawPostings to Core's ingest endpoint. A module supplies only a fetch
// function that emits postings; the kit does the rest.
package scraperkit

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

const contractVersion = "1.1.0"

// RawPosting is Headhunter's canonical ingest shape, identical across modules.
type RawPosting struct {
	URL      string          `json:"url"`
	Title    string          `json:"title"`
	Company  string          `json:"company"`
	ATS      string          `json:"ats,omitempty"` // source board (v1.1); stamped centrally by Main
	Location string          `json:"location,omitempty"`
	Comp     string          `json:"comp,omitempty"`
	PostedAt string          `json:"postedAt,omitempty"`
	Raw      json.RawMessage `json:"raw,omitempty"`
}

// Config is passed to a module's fetch function.
type Config struct {
	ATS       string
	Keywords  []string // lowercased ROLE_KEYWORDS ("" list = match everything)
	IngestURL string
}

// Client is the shared HTTP client (modules use it for custom requests).
var Client = &http.Client{Timeout: 30 * time.Second}

// Env returns env var k, or def when unset/empty.
func Env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// GetJSON GETs url (with optional headers) and decodes the body into v.
// A non-200 is an error.
func GetJSON(url string, headers map[string]string, v any) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	resp, err := Client.Do(req)
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

// MatchAny reports whether lowerTitle contains any keyword (empty list matches).
func MatchAny(lowerTitle string, kws []string) bool {
	if len(kws) == 0 {
		return true
	}
	for _, k := range kws {
		if k != "" && strings.Contains(lowerTitle, k) {
			return true
		}
	}
	return false
}

// Main drives a scraper module: it prints the handshake, builds Config from the
// environment, runs fetch (which emits postings), applies the keyword filter and
// URL dedup, POSTs the batch to the ingest endpoint, logs the total, and sets the
// process exit code. emit returns true when a posting was accepted (passed the
// filter and wasn't a dup) so modules can keep per-source counts.
func Main(ats string, fetch func(cfg Config, emit func(RawPosting) bool) error) {
	cfg := Config{ATS: ats, IngestURL: Env("CORE_INGEST_URL", "http://headhunter-core.headhunter.svc.cluster.local:8080/api/scan/ingest")}
	for _, w := range strings.Split(os.Getenv("ROLE_KEYWORDS"), ",") {
		if w = strings.ToLower(strings.TrimSpace(w)); w != "" {
			cfg.Keywords = append(cfg.Keywords, w)
		}
	}

	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
		"ats": ats, "contractVersion": contractVersion, "capabilities": []string{"http-json"},
	})

	seen := map[string]bool{}
	var all []RawPosting
	emit := func(p RawPosting) bool {
		if p.URL == "" || seen[p.URL] {
			return false
		}
		if !MatchAny(strings.ToLower(p.Title), cfg.Keywords) {
			return false
		}
		seen[p.URL] = true
		p.ATS = ats // v1.1: stamp the source board so Core attributes sightings per ATS
		all = append(all, p)
		return true
	}

	if err := fetch(cfg, emit); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] fetch error: %v\n", ats, err)
	}
	fmt.Fprintf(os.Stderr, "[%s] total matched: %d\n", ats, len(all))
	if len(all) == 0 {
		return
	}

	body, _ := json.Marshal(all)
	resp, err := Client.Post(cfg.IngestURL, "application/json", bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] ingest error: %v\n", ats, err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	fmt.Fprintf(os.Stderr, "[%s] ingest -> %d %s\n", ats, resp.StatusCode, strings.TrimSpace(string(out)))
	if resp.StatusCode >= 300 {
		os.Exit(1)
	}
}
