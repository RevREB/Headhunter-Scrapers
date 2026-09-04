package main

import (
	"encoding/json"
	"testing"
)

func TestAppleDetailsURL(t *testing.T) {
	cases := []struct {
		name, id, slug, want string
	}{
		{"id and slug", "200123456", "senior-infra-engineer", "https://jobs.apple.com/en-us/details/200123456/senior-infra-engineer"},
		{"id no slug", "200123456", "", "https://jobs.apple.com/en-us/details/200123456"},
		{"slug whitespace only", "200123456", "   ", "https://jobs.apple.com/en-us/details/200123456"},
		{"empty id", "", "some-slug", ""},
		{"trims id and slug", "  200987  ", "  cloud-lead  ", "https://jobs.apple.com/en-us/details/200987/cloud-lead"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := detailsURL(c.id, c.slug); got != c.want {
				t.Fatalf("detailsURL(%q,%q) = %q, want %q", c.id, c.slug, got, c.want)
			}
		})
	}
}

func TestAppleSplitQueries(t *testing.T) {
	got := splitQueries(" infrastructure , platform ,, ,site reliability ")
	want := []string{"infrastructure", "platform", "site reliability"}
	if len(got) != len(want) {
		t.Fatalf("len = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if q := splitQueries("   "); q != nil {
		t.Fatalf("empty input should yield nil, got %v", q)
	}
}

func TestAppleSplitKeywords(t *testing.T) {
	got := splitKeywords(" Kubernetes , Platform , , DEVOPS ")
	want := []string{"kubernetes", "platform", "devops"}
	if len(got) != len(want) {
		t.Fatalf("len = %d (%v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAppleMatchesKeywords(t *testing.T) {
	if !matchesKeywords("Anything", nil) {
		t.Fatal("empty keyword set should match all")
	}
	if !matchesKeywords("Senior Platform Engineer", []string{"platform"}) {
		t.Fatal("expected match on 'platform'")
	}
	if !matchesKeywords("SITE RELIABILITY ENGINEER", []string{"site reliability"}) {
		t.Fatal("expected case-insensitive match")
	}
	if matchesKeywords("Retail Specialist", []string{"kubernetes", "devops"}) {
		t.Fatal("did not expect a match")
	}
}

func TestApplePickTitle(t *testing.T) {
	if got := pickTitle(appleSearchResult{PostingTitle: "A", PositionTitle: "B", Title: "C"}); got != "A" {
		t.Fatalf("postingTitle should win, got %q", got)
	}
	if got := pickTitle(appleSearchResult{PositionTitle: "B", Title: "C"}); got != "B" {
		t.Fatalf("positionTitle fallback failed, got %q", got)
	}
	if got := pickTitle(appleSearchResult{Title: "C"}); got != "C" {
		t.Fatalf("title fallback failed, got %q", got)
	}
	if got := pickTitle(appleSearchResult{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestApplePickID(t *testing.T) {
	if got := pickID(appleSearchResult{ID: "111", PositionID: "222"}); got != "222" {
		t.Fatalf("positionId should win, got %q", got)
	}
	if got := pickID(appleSearchResult{ID: "111"}); got != "111" {
		t.Fatalf("id fallback failed, got %q", got)
	}
	if got := pickID(appleSearchResult{}); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestApplePickDate(t *testing.T) {
	if got := pickDate(appleSearchResult{PostDateInGMT: "2026-08-01", PostingDate: "2026-07-01"}); got != "2026-08-01" {
		t.Fatalf("postDateInGMT should win, got %q", got)
	}
	if got := pickDate(appleSearchResult{PostingDate: "2026-07-01"}); got != "2026-07-01" {
		t.Fatalf("postingDate fallback failed, got %q", got)
	}
}

func TestAppleParseLocations(t *testing.T) {
	// Array of {name}.
	arr := json.RawMessage(`[{"name":"Cupertino"},{"name":"Austin"}]`)
	if got := parseLocations(arr); got != "Cupertino, Austin" {
		t.Fatalf("array parse = %q, want %q", got, "Cupertino, Austin")
	}
	// Bare string.
	str := json.RawMessage(`"Remote USA"`)
	if got := parseLocations(str); got != "Remote USA" {
		t.Fatalf("string parse = %q, want %q", got, "Remote USA")
	}
	// Empty / missing.
	if got := parseLocations(nil); got != "" {
		t.Fatalf("nil parse = %q, want empty", got)
	}
	// Unexpected shape does not panic and yields empty.
	if got := parseLocations(json.RawMessage(`12345`)); got != "" {
		t.Fatalf("numeric parse = %q, want empty", got)
	}
}

func TestAppleSearchRequestBody(t *testing.T) {
	b, err := searchRequestBody("kubernetes", 3)
	if err != nil {
		t.Fatalf("searchRequestBody error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("body is not valid JSON: %v", err)
	}
	if decoded["query"] != "kubernetes" {
		t.Fatalf("query = %v, want kubernetes", decoded["query"])
	}
	if decoded["page"].(float64) != 3 {
		t.Fatalf("page = %v, want 3", decoded["page"])
	}
	if decoded["locale"] != "en-us" {
		t.Fatalf("locale = %v, want en-us", decoded["locale"])
	}
	filters, ok := decoded["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters missing or wrong type: %v", decoded["filters"])
	}
	locs, ok := filters["postingpostLocation"].([]any)
	if !ok || len(locs) != 1 || locs[0] != "postLocation-USA" {
		t.Fatalf("postingpostLocation = %v, want [postLocation-USA]", filters["postingpostLocation"])
	}
}

func TestAppleResponseDecodeTolerant(t *testing.T) {
	// Missing/partial fields must not error.
	raw := `{"res":{"totalRecords":1,"searchResults":[{"id":"200000001"}]}}`
	var ar appleResponse
	if err := json.Unmarshal([]byte(raw), &ar); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if len(ar.Res.SearchResults) != 1 {
		t.Fatalf("expected 1 result, got %d", len(ar.Res.SearchResults))
	}
	if got := pickID(ar.Res.SearchResults[0]); got != "200000001" {
		t.Fatalf("pickID = %q", got)
	}
}
