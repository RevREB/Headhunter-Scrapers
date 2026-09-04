package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// Contract smoke test: a scraper must print a valid Handshake as its first line.
// Full fixture-based tests (recorded response -> asserted RawPostings) live
// alongside each real scraper under fixtures/<ats>/.
func TestRunEmitsHandshakeFirst(t *testing.T) {
	var buf bytes.Buffer
	if err := run(portalConfig{ATS: ats}, &buf); err != nil {
		t.Fatalf("run: %v", err)
	}
	var h handshake
	if err := json.NewDecoder(&buf).Decode(&h); err != nil {
		t.Fatalf("first stdout line is not a Handshake: %v", err)
	}
	if h.ATS != ats || h.ContractVersion != contractVersion {
		t.Errorf("handshake = %+v", h)
	}
}
