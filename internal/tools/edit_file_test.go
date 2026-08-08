package tools

import (
	"strings"
	"testing"
)

func TestApplyBestEffortEditTrimmedReplacementPreservesBytesOutsideRange(t *testing.T) {
	content := "alpha\n  target one\n  target two\nomega\n"
	updated, match, err := applyBestEffortEdit(content, "target one\ntarget two", "replacement")
	if err != nil {
		t.Fatalf("applyBestEffortEdit() error = %v", err)
	}
	if match.Strategy != "line_trimmed" {
		t.Fatalf("strategy = %q, want line_trimmed", match.Strategy)
	}
	want := "alpha\nreplacement\nomega\n"
	if updated != want {
		t.Fatalf("updated content = %q, want %q", updated, want)
	}
}

func TestApplyBestEffortEditFuzzyMatchesFinalWindow(t *testing.T) {
	content := "intro\nalpha beta gamma\nomega delta zeta\n"
	oldString := "alpha beta gamma\nomega delta theta\n"

	updated, match, err := applyBestEffortEdit(content, oldString, "fixed")
	if err != nil {
		t.Fatalf("applyBestEffortEdit() error = %v", err)
	}
	if !strings.HasPrefix(match.Strategy, "fuzzy_line_window") {
		t.Fatalf("strategy = %q, want fuzzy_line_window", match.Strategy)
	}
	want := "intro\nfixed\n"
	if updated != want {
		t.Fatalf("updated content = %q, want %q", updated, want)
	}
}

func TestApplyBestEffortEditFuzzyMatchesWholeFileWindow(t *testing.T) {
	content := "alpha beta gamma\nomega delta zeta\n"
	oldString := "alpha beta gamma\nomega delta theta\n"

	updated, match, err := applyBestEffortEdit(content, oldString, "fixed")
	if err != nil {
		t.Fatalf("applyBestEffortEdit() error = %v", err)
	}
	if !strings.HasPrefix(match.Strategy, "fuzzy_line_window") {
		t.Fatalf("strategy = %q, want fuzzy_line_window", match.Strategy)
	}
	if updated != "fixed\n" {
		t.Fatalf("updated content = %q, want %q", updated, "fixed\n")
	}
}
