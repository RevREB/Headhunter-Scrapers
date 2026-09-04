package main

import "testing"

func TestMatchAny(t *testing.T) {
	kw := []string{"platform", "sre", "infrastructure"}
	if !matchAny("senior platform engineer", kw) {
		t.Error("should match platform")
	}
	if matchAny("staff accountant", kw) {
		t.Error("should not match")
	}
	if matchAny("anything", nil) {
		t.Error("empty keywords should not match here")
	}
}
