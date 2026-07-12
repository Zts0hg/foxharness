package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureFilesCreatesPlanAndTodoButNotLegacyMemory(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	store := NewSessionStore(workDir, sessionDir)

	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(sessionDir, "PLAN.md")); err != nil {
		t.Fatalf("PLAN.md was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "TODO.md")); err != nil {
		t.Fatalf("TODO.md was not created: %v", err)
	}

	if _, err := os.Stat(filepath.Join(workDir, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("legacy {workDir}/MEMORY.md must not be created (REQ-017); stat err = %v", err)
	}
}

func TestEnsureFilesIsIdempotent(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	store := NewSessionStore(workDir, sessionDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() first call error = %v", err)
	}
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() second call error = %v", err)
	}
}

func TestReplacePlanPreservesExactContent(t *testing.T) {
	sessionDir := t.TempDir()
	store := NewSessionStore(t.TempDir(), sessionDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}

	want := "\n# Proposed plan\n\n- keep leading newline\n- no trailing newline"
	if err := store.ReplacePlan(want); err != nil {
		t.Fatalf("ReplacePlan() error = %v", err)
	}
	data, err := os.ReadFile(store.PlanPath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != want {
		t.Fatalf("PLAN.md = %q, want exact %q", got, want)
	}

	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".PLAN-") {
			t.Fatalf("temporary plan file was not removed: %s", entry.Name())
		}
	}
}

func TestReplacePlanReplacesPreviousProposal(t *testing.T) {
	store := NewSessionStore(t.TempDir(), t.TempDir())
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	if err := store.ReplacePlan("first"); err != nil {
		t.Fatalf("ReplacePlan(first) error = %v", err)
	}
	if err := store.ReplacePlan("second"); err != nil {
		t.Fatalf("ReplacePlan(second) error = %v", err)
	}
	data, err := os.ReadFile(store.PlanPath())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(data); got != "second" {
		t.Fatalf("PLAN.md = %q, want second", got)
	}
}
