package main

import "testing"

func TestParsePostedDate(t *testing.T) {
	if got := parsePostedDate("July  3, 2026"); got != "2026-07-03T00:00:00Z" {
		t.Errorf("got %q", got)
	}
	if parsePostedDate("") != "" || parsePostedDate("not a date") != "" {
		t.Error("bad dates should be empty")
	}
}

func TestSearchURL(t *testing.T) {
	u := searchURL("sre", "USA", 100)
	for _, want := range []string{"base_query=sre", "offset=100", "normalized_country_code%5B%5D=USA"} {
		if !contains(u, want) {
			t.Errorf("url %q missing %q", u, want)
		}
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (indexOf(s, sub) >= 0) }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
