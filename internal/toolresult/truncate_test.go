package toolresult

import (
	"strings"
	"testing"
)

func TestTruncateToCap_UnderCap(t *testing.T) {
	in := strings.Repeat("a", 100)
	got := TruncateToCap(in)
	if got != in {
		t.Fatalf("TruncateToCap returned modified content for input < cap")
	}
}

func TestTruncateToCap_ExactlyAtCap(t *testing.T) {
	in := strings.Repeat("b", MaxToolResultBytes)
	got := TruncateToCap(in)
	if got != in {
		t.Fatalf("TruncateToCap modified content of exact cap size")
	}
}

func TestTruncateToCap_OverCap(t *testing.T) {
	in := strings.Repeat("c", MaxToolResultBytes+5000)
	got := TruncateToCap(in)
	if len(got) <= MaxToolResultBytes {
		t.Fatalf("TruncateToCap should not silently shorten below cap")
	}
	if !strings.HasPrefix(got, strings.Repeat("c", MaxToolResultBytes)) {
		t.Fatalf("truncated content should retain the first %d bytes verbatim", MaxToolResultBytes)
	}
	if !strings.Contains(got, "[truncated at 400KB") {
		t.Fatalf("truncated content should contain truncation notice, got tail: %q", got[len(got)-200:])
	}
}

func TestTruncateToCap_SixHundredKB(t *testing.T) {
	in := strings.Repeat("d", 600000)
	got := TruncateToCap(in)
	if !strings.HasPrefix(got, strings.Repeat("d", MaxToolResultBytes)) {
		t.Fatalf("truncated 600KB content should keep first 400KB intact")
	}
	if !strings.Contains(got, "original size:") {
		t.Fatalf("truncation notice should mention original size")
	}
}
