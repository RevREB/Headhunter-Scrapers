package main

import (
	"encoding/json"
	"testing"
)

func TestMatchAny(t *testing.T) {
	kw := []string{"platform", "sre", "infrastructure"}
	if !matchAny("senior platform engineer", kw) {
		t.Error("should match platform")
	}
	if !matchAny("site reliability (sre)", kw) {
		t.Error("should match sre")
	}
	if matchAny("staff accountant", kw) {
		t.Error("should not match")
	}
	if matchAny("anything", nil) {
		t.Error("empty keywords should not match here")
	}
}

func TestCompanyName(t *testing.T) {
	cases := map[string]string{
		"plaid":  "Plaid",
		"brex":   "Brex",
		"a":      "A",
		"":       "",
		"notion": "Notion",
	}
	for in, want := range cases {
		if got := companyName(in); got != want {
			t.Errorf("companyName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPostingsURL(t *testing.T) {
	got := postingsURL("plaid")
	want := "https://api.lever.co/v0/postings/plaid?mode=json"
	if got != want {
		t.Errorf("postingsURL = %q, want %q", got, want)
	}
}

func TestPostedAt(t *testing.T) {
	if got := postedAt(0); got != "" {
		t.Errorf("postedAt(0) = %q, want empty", got)
	}
	// 1_700_000_000_000 ms = 2023-11-14T22:13:20Z
	if got := postedAt(1_700_000_000_000); got != "2023-11-14T22:13:20Z" {
		t.Errorf("postedAt(1700000000000) = %q, want 2023-11-14T22:13:20Z", got)
	}
}

// TestRawPreservesFullPosting guards that Raw carries the verbatim posting
// JSON — including fields we do not map — rather than a lossy re-marshal.
func TestRawPreservesFullPosting(t *testing.T) {
	body := `[{"text":"SRE","hostedUrl":"https://x/y","categories":{"location":"Remote"},"createdAt":1700000000000,"id":"abc","description":"keep me"}]`

	var items []json.RawMessage
	if err := json.Unmarshal([]byte(body), &items); err != nil {
		t.Fatalf("decode array: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}

	var p leverPosting
	if err := json.Unmarshal(items[0], &p); err != nil {
		t.Fatalf("decode posting: %v", err)
	}
	if p.Text != "SRE" || p.HostedURL != "https://x/y" || p.Categories.Location != "Remote" {
		t.Errorf("mapping wrong: %+v", p)
	}

	var got map[string]any
	if err := json.Unmarshal(items[0], &got); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	if got["id"] != "abc" {
		t.Errorf("Raw dropped id: %v", got["id"])
	}
	if got["description"] != "keep me" {
		t.Errorf("Raw dropped description: %v", got["description"])
	}
}
