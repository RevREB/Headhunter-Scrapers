package main

import (
	"encoding/json"
	"testing"
)

func TestDetailsURL(t *testing.T) {
	if detailsURL("200", "sre-role") != "https://jobs.apple.com/en-us/details/200/sre-role" {
		t.Error("with slug")
	}
	if detailsURL("200", "") != "https://jobs.apple.com/en-us/details/200" {
		t.Error("no slug")
	}
	if detailsURL("", "x") != "" {
		t.Error("no id -> empty")
	}
}

func TestParseLocations(t *testing.T) {
	if got := parseLocations(json.RawMessage(`[{"name":"Cupertino"},{"name":"Austin"}]`)); got != "Cupertino, Austin" {
		t.Errorf("array: %q", got)
	}
	if got := parseLocations(json.RawMessage(`"Remote"`)); got != "Remote" {
		t.Errorf("string: %q", got)
	}
	if parseLocations(nil) != "" {
		t.Error("nil -> empty")
	}
}
