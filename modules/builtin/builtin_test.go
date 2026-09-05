package main

import (
	"strings"
	"testing"
)

// Fixtures mirror Built In's real markup: JSON-LD in a <script> whose type has
// the '+' HTML-entity-encoded (application/ld&#x2B;json), wrapped in an @graph.

const listFixture = `<html><head>
<script type="application/ld&#x2B;json">
{"@context":"https://schema.org","@graph":[
 {"@type":"CollectionPage"},
 {"@type":"ItemList","itemListElement":[
   {"@type":"ListItem","position":1,"name":"Staff Platform Engineer","url":"https://builtin.com/job/staff-platform-engineer/123"},
   {"@type":"ListItem","position":2,"name":"Marketing Coordinator","url":"https://builtin.com/job/marketing-coordinator/456"}
 ]}
]}
</script></head></html>`

const jobFixture = `<html><head>
<script type="application/ld&#x2B;json">
{"@context":"https://schema.org","@graph":[
 {"@type":"BreadcrumbList"},
 {"@type":"JobPosting","title":"Staff Platform Engineer","datePosted":"2026-09-05",
  "description":"<strong>Job Description:</strong><br>Build platforms.",
  "employmentType":"FULL_TIME","directApply":false,
  "hiringOrganization":{"@type":"Organization","name":"Acme","sameAs":"https://builtin.com/company/acme"},
  "jobLocation":{"@type":"Place","address":{"@type":"PostalAddress","addressLocality":"Peoria","addressRegion":"Illinois","addressCountry":"USA"}},
  "baseSalary":{"@type":"MonetaryAmount","currency":"USD","value":{"@type":"QuantitativeValue","minValue":150000,"maxValue":220000,"unitText":"YEAR"}}},
 {"@type":"Organization","name":"Acme"}
]}
</script></head><body>...</body></html>`

// remoteJobFixture has no physical jobLocation, only applicantLocationRequirements.
const remoteJobFixture = `<script type="application/ld&#x2B;json">
{"@graph":[{"@type":"JobPosting","title":"Remote SRE","datePosted":"2026-09-01T12:00:00+00:00",
 "hiringOrganization":{"@type":"Organization","name":"Globex"},
 "applicantLocationRequirements":{"@type":"Country","name":"USA"},
 "baseSalary":{"@type":"MonetaryAmount","currency":"USD","value":{"minValue":180000,"unitText":"HOUR"}}}]}
</script>`

func TestParseListItems(t *testing.T) {
	items := parseListItems(listFixture)
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d", len(items))
	}
	if items[0].URL != "https://builtin.com/job/staff-platform-engineer/123" || items[0].Name != "Staff Platform Engineer" {
		t.Fatalf("bad first item: %+v", items[0])
	}
}

func TestParseJob(t *testing.T) {
	rp, ok := parseJob(jobFixture, "https://builtin.com/job/staff-platform-engineer/123", "fallback")
	if !ok {
		t.Fatal("expected a JobPosting")
	}
	if rp.Title != "Staff Platform Engineer" {
		t.Errorf("title=%q", rp.Title)
	}
	if rp.Company != "Acme" {
		t.Errorf("company=%q", rp.Company)
	}
	if rp.Location != "Peoria, Illinois" {
		t.Errorf("location=%q", rp.Location)
	}
	if rp.Comp != "USD $150,000–$220,000/yr" {
		t.Errorf("comp=%q", rp.Comp)
	}
	if rp.PostedAt != "2026-09-05T00:00:00Z" {
		t.Errorf("postedAt=%q", rp.PostedAt)
	}
	// The full structured record — including the discovery signal — is kept in Raw.
	if !strings.Contains(string(rp.Raw), "builtin.com/company/acme") {
		t.Error("hiringOrganization.sameAs (discovery signal) missing from Raw")
	}
	if !strings.Contains(string(rp.Raw), "Build platforms") {
		t.Error("JD description missing from Raw")
	}
}

func TestParseJobRemote(t *testing.T) {
	rp, ok := parseJob(remoteJobFixture, "https://builtin.com/job/remote-sre/789", "")
	if !ok {
		t.Fatal("expected a JobPosting")
	}
	if rp.Location != "Remote (USA)" {
		t.Errorf("location=%q, want Remote (USA)", rp.Location)
	}
	if rp.Comp != "USD $180,000/hr" {
		t.Errorf("comp=%q", rp.Comp)
	}
	if rp.PostedAt != "2026-09-01T12:00:00+00:00" {
		t.Errorf("postedAt=%q (RFC3339 should pass through)", rp.PostedAt)
	}
}

func TestMoney(t *testing.T) {
	for in, want := range map[float64]string{0: "$0", 100: "$100", 1500: "$1,500", 150000: "$150,000", 1234567: "$1,234,567"} {
		if got := money(in); got != want {
			t.Errorf("money(%v)=%q, want %q", in, got, want)
		}
	}
}
