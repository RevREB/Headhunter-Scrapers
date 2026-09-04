package main

import "testing"

func TestMatchAny(t *testing.T) {
	kws := []string{"sre", "platform", "site reliability"}
	cases := []struct {
		title string
		kws   []string
		want  bool
	}{
		{"Senior SRE, Infrastructure", kws, true},
		{"Staff Platform Engineer", kws, true},
		{"Site Reliability Engineer II", kws, true},
		{"Frontend React Developer", kws, false},
		{"Accountant", kws, false},
		{"anything at all", nil, true},        // empty keywords -> match everything
		{"anything at all", []string{}, true}, // empty slice -> match everything
		{"platform", []string{""}, false},     // only empty keyword -> no real match
		{"SITE reliability lead", kws, true},  // case-insensitive
	}
	for _, tc := range cases {
		if got := matchAny(tc.title, tc.kws); got != tc.want {
			t.Errorf("matchAny(%q, %v) = %v, want %v", tc.title, tc.kws, got, tc.want)
		}
	}
}

func TestParseCompanies(t *testing.T) {
	in := "nvidia.wd5.myworkdayjobs.com|nvidia|NVIDIAExternalCareerSite|NVIDIA," +
		"boeing.wd1.myworkdayjobs.com|boeing|external_careers," + // no display -> tenant
		"  bad-tuple-only-two|parts  ," + // malformed -> skipped
		"|empty|host|X," + // empty host -> skipped
		"" // trailing empty -> skipped

	got := parseCompanies(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 companies, got %d: %+v", len(got), got)
	}

	if got[0].Host != "nvidia.wd5.myworkdayjobs.com" || got[0].Tenant != "nvidia" ||
		got[0].Site != "NVIDIAExternalCareerSite" || got[0].Display != "NVIDIA" {
		t.Errorf("company[0] mismatch: %+v", got[0])
	}

	// Display should fall back to tenant when omitted.
	if got[1].Display != "boeing" {
		t.Errorf("company[1] display fallback = %q, want %q", got[1].Display, "boeing")
	}
	if got[1].Site != "external_careers" {
		t.Errorf("company[1] site = %q, want %q", got[1].Site, "external_careers")
	}
}

func TestParseKeywords(t *testing.T) {
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
