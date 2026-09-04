package scraperkit

import "testing"

func TestMatchAny(t *testing.T) {
	kw := []string{"platform", "sre", "infrastructure"}
	if !MatchAny("senior platform engineer", kw) {
		t.Error("should match platform")
	}
	if MatchAny("staff accountant", kw) {
		t.Error("should not match")
	}
	if !MatchAny("anything at all", nil) {
		t.Error("empty keywords should match everything")
	}
}
