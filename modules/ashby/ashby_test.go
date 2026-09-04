package main

import "testing"

func TestTitleCaseBoard(t *testing.T) {
	for in, want := range map[string]string{"perplexity-ai": "Perplexity Ai", "openai": "Openai", "together-ai": "Together Ai"} {
		if got := titleCaseBoard(in); got != want {
			t.Errorf("titleCaseBoard(%q)=%q want %q", in, got, want)
		}
	}
}

func TestJobURL(t *testing.T) {
	if jobURL("openai", "https://x/y", "1") != "https://x/y" {
		t.Error("prefer jobUrl")
	}
	if jobURL("openai", "", "1") != "https://jobs.ashbyhq.com/openai/1" {
		t.Error("fallback")
	}
	if jobURL("openai", "", "") != "" {
		t.Error("empty")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if firstNonEmpty("", "b", "c") != "b" {
		t.Error("bad")
	}
	if firstNonEmpty("", "") != "" {
		t.Error("bad empty")
	}
}
