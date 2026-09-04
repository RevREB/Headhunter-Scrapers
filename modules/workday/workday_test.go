package main

import "testing"

func TestParseCompanies(t *testing.T) {
	c := parseCompanies("a.wd5.myworkdayjobs.com|a|SiteA|Acme, bad|tuple, b.wd1.x|b|SiteB")
	if len(c) != 2 {
		t.Fatalf("got %d, want 2 (bad tuple skipped)", len(c))
	}
	if c[0].Display != "Acme" || c[0].Tenant != "a" || c[0].Site != "SiteA" {
		t.Errorf("c0 = %+v", c[0])
	}
	if c[1].Display != "b" { // display falls back to tenant
		t.Errorf("c1 display = %q, want b", c[1].Display)
	}
}
