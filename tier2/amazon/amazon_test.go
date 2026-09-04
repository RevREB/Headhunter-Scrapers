package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestAmazonMatchAny(t *testing.T) {
	kws := []string{"sre", "platform", "site reliability", "infrastructure"}
	cases := []struct {
		title string
		kws   []string
		want  bool
	}{
		{"Site Reliability Engineer, AWS", kws, true},
		{"Senior Platform Engineer", kws, true},
		{"Infrastructure Engineer II", kws, true},
		{"SRE, Networking", kws, true},
		{"Frontend React Developer", kws, false},
		{"Warehouse Associate", kws, false},
		{"anything at all", nil, true},        // no keywords -> match everything
		{"anything at all", []string{}, true}, // empty slice -> match everything
		{"platform", []string{""}, false},     // only empty keyword -> no real match
		{"PLATFORM Engineer", kws, true},      // case-insensitive
	}
	for _, tc := range cases {
		if got := matchAny(tc.title, tc.kws); got != tc.want {
			t.Errorf("matchAny(%q, %v) = %v, want %v", tc.title, tc.kws, got, tc.want)
		}
	}
}

func TestAmazonSplitQueries(t *testing.T) {
	got := splitQueries("site reliability engineer, platform engineer ,, devops,")
	want := []string{"site reliability engineer", "platform engineer", "devops"}
	if len(got) != len(want) {
		t.Fatalf("expected %d queries, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("query[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(splitQueries("")) != 0 {
		t.Errorf("empty input should yield no queries")
	}
	if len(splitQueries("  ,  , ")) != 0 {
		t.Errorf("whitespace-only input should yield no queries")
	}
}

func TestAmazonParseKeywords(t *testing.T) {
	got := parseKeywords("Infrastructure, Platform ,, SRE,")
	want := []string{"infrastructure", "platform", "sre"}
	if len(got) != len(want) {
		t.Fatalf("expected %d keywords, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("keyword[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if len(parseKeywords("")) != 0 {
		t.Errorf("empty input should yield no keywords")
	}
}

func TestAmazonSearchURL(t *testing.T) {
	raw := searchURL("site reliability engineer", "USA", 200)
	if !strings.HasPrefix(raw, "https://www.amazon.jobs/en/search.json?") {
		t.Fatalf("unexpected URL prefix: %s", raw)
	}
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	q := u.Query()
	checks := map[string]string{
		"base_query":                "site reliability engineer",
		"loc_query":                 "",
		"sort":                      "recent",
		"result_limit":              "100",
		"offset":                    "200",
		"normalized_country_code[]": "USA",
	}
	for k, want := range checks {
		if got := q.Get(k); got != want {
			t.Errorf("query %q = %q, want %q", k, got, want)
		}
	}
}

func TestAmazonParsePostedDate(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"July  3, 2026", "2026-07-03T00:00:00Z"}, // double space collapses
		{"January 2, 2006", "2006-01-02T00:00:00Z"},
		{"December 25, 2025", "2025-12-25T00:00:00Z"},
		{"  March   9,   2026 ", "2026-03-09T00:00:00Z"}, // messy whitespace
		{"", ""},           // empty -> empty
		{"not a date", ""}, // garbage -> empty
		{"2026-07-03", ""}, // wrong layout -> empty
	}
	for _, tc := range cases {
		if got := parsePostedDate(tc.in); got != tc.want {
			t.Errorf("parsePostedDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
