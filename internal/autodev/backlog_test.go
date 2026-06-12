package autodev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBacklog(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "BACKLOG.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write backlog: %v", err)
	}
	return path
}

func TestParseWellFormedItems(t *testing.T) {
	path := writeBacklog(t, `# Backlog

## [feature] Engine writes durable discoveries to MEMORY.md during runs

**Priority**: high
**Status**: pending
**Description**: During an agent run, the Engine should persist durable facts.

Memory-worthy examples: stable project conventions.

## [fix] Repair the flaky retry test

**Priority**: medium
**Status**: in-progress
**Description**: The retry test fails intermittently on CI.
`)

	items, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	first := items[0]
	if first.Type != "feature" {
		t.Errorf("items[0].Type = %q, want feature", first.Type)
	}
	if first.Title != "Engine writes durable discoveries to MEMORY.md during runs" {
		t.Errorf("items[0].Title = %q", first.Title)
	}
	if first.Priority != PriorityHigh {
		t.Errorf("items[0].Priority = %q, want high", first.Priority)
	}
	if first.Status != StatusPending {
		t.Errorf("items[0].Status = %q, want pending", first.Status)
	}
	if !strings.Contains(first.Description, "persist durable facts") {
		t.Errorf("items[0].Description = %q, want requirement text", first.Description)
	}
	if !strings.Contains(first.Description, "stable project conventions") {
		t.Errorf("items[0].Description = %q, want continuation paragraph included", first.Description)
	}

	second := items[1]
	if second.Type != "fix" {
		t.Errorf("items[1].Type = %q, want fix", second.Type)
	}
	if second.Priority != PriorityMedium {
		t.Errorf("items[1].Priority = %q, want medium", second.Priority)
	}
	if second.Status != StatusInProgress {
		t.Errorf("items[1].Status = %q, want in-progress", second.Status)
	}
}

func TestParseMissingStatusDefaultsToPending(t *testing.T) {
	path := writeBacklog(t, `## [feature] No status here

**Priority**: high
**Description**: Something.
`)

	items, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Status != StatusPending {
		t.Errorf("Status = %q, want pending default", items[0].Status)
	}
}

func TestParseMissingPriorityDefaultsToLow(t *testing.T) {
	path := writeBacklog(t, `## [feature] No priority here

**Status**: pending
**Description**: Something.
`)

	items, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Priority != PriorityLow {
		t.Errorf("Priority = %q, want low default", items[0].Priority)
	}
}

func TestParseHeadingWithoutTypeBracket(t *testing.T) {
	path := writeBacklog(t, `## Plain title without a type

**Priority**: low
**Description**: Something.
`)

	items, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Type != "" {
		t.Errorf("Type = %q, want empty", items[0].Type)
	}
	if items[0].Title != "Plain title without a type" {
		t.Errorf("Title = %q", items[0].Title)
	}
}

func TestParseEmptyFileYieldsNoItems(t *testing.T) {
	path := writeBacklog(t, "")

	items, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("len(items) = %d, want 0", len(items))
	}
}

func TestParseMissingFileReturnsError(t *testing.T) {
	if _, err := Parse(filepath.Join(t.TempDir(), "nope.md")); err == nil {
		t.Fatal("Parse returned nil error for missing file, want error")
	}
}
