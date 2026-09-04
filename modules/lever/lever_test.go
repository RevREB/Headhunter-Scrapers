package main

import "testing"

func TestCompanyName(t *testing.T) {
	for in, want := range map[string]string{"plaid": "Plaid", "brex": "Brex", "a": "A", "": ""} {
		if got := companyName(in); got != want {
			t.Errorf("companyName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPostedAt(t *testing.T) {
	if postedAt(0) != "" {
		t.Error("0 -> empty")
	}
	if got := postedAt(1_700_000_000_000); got != "2023-11-14T22:13:20Z" {
		t.Errorf("got %q", got)
	}
}

func TestPostingsURL(t *testing.T) {
	if postingsURL("plaid") != "https://api.lever.co/v0/postings/plaid?mode=json" {
		t.Error("bad url")
	}
}
