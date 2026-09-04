// Command example is a Tier-2 Headhunter scraper stub: a one-shot batch program
// that fetches raw postings from a single ATS and prints them to stdout.
//
// Wire contract (canonical Go types live in
// github.com/RevREB/Headhunter-Core/pkg/scraper — a real Go scraper should
// import them; this stub inlines the minimal shapes so it builds standalone):
//
//  1. read PORTAL_CONFIG (JSON) from the environment
//  2. print a Handshake as the FIRST line of stdout
//  3. print a JSON array of RawPosting, then exit 0 (non-zero on failure)
package main

import (
	"encoding/json"
	"io"
	"os"
)

const (
	ats             = "example"
	contractVersion = "1.0.0"
)

type portalConfig struct {
	ATS   string         `json:"ats"`
	Query map[string]any `json:"query,omitempty"`
}

type handshake struct {
	ATS             string   `json:"ats"`
	ContractVersion string   `json:"contractVersion"`
	Capabilities    []string `json:"capabilities,omitempty"`
}

type rawPosting struct {
	URL      string `json:"url"`
	Title    string `json:"title"`
	Company  string `json:"company"`
	Location string `json:"location,omitempty"`
	PostedAt string `json:"postedAt,omitempty"`
}

// run emits the handshake then the postings; separated from main for testing.
func run(cfg portalConfig, w io.Writer) error {
	enc := json.NewEncoder(w)
	if err := enc.Encode(handshake{
		ATS: ats, ContractVersion: contractVersion, Capabilities: []string{"http"},
	}); err != nil {
		return err
	}
	// TODO: fetch from the real ATS (using cfg) and populate postings.
	postings := []rawPosting{}
	return enc.Encode(postings)
}

func main() {
	var cfg portalConfig
	if s := os.Getenv("PORTAL_CONFIG"); s != "" {
		_ = json.Unmarshal([]byte(s), &cfg)
	}
	if err := run(cfg, os.Stdout); err != nil {
		os.Exit(1)
	}
}
