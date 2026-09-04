package main

import "testing"

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
	if matchAny("has empty kw", []string{""}) {
		t.Error("empty-string keyword should not match")
	}
}

func TestTitleCaseBoard(t *testing.T) {
	cases := map[string]string{
		"perplexity-ai": "Perplexity Ai",
		"together-ai":   "Together Ai",
		"openai":        "Openai",
		"linear":        "Linear",
		"foo_bar":       "Foo Bar",
		"":              "",
	}
	for in, want := range cases {
		if got := titleCaseBoard(in); got != want {
			t.Errorf("titleCaseBoard(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", "", "2026-01-02"); got != "2026-01-02" {
		t.Errorf("got %q, want the third value", got)
	}
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Errorf("got %q, want a", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestJobURL(t *testing.T) {
	if got := jobURL("linear", "https://jobs.ashbyhq.com/linear/abc", "abc"); got != "https://jobs.ashbyhq.com/linear/abc" {
		t.Errorf("should prefer jobUrl field, got %q", got)
	}
	if got := jobURL("linear", "", "abc123"); got != "https://jobs.ashbyhq.com/linear/abc123" {
		t.Errorf("should build from id, got %q", got)
	}
	if got := jobURL("linear", "", ""); got != "" {
		t.Errorf("no url and no id should be empty, got %q", got)
	}
}
