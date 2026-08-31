package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionStoreOwnsWorkingMemoryLifecycle(t *testing.T) {
	sessionDir := t.TempDir()
	store := NewSessionStore(t.TempDir(), sessionDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}

	wantInitial := "# Working Memory\n\n## Goal\n\nNot recorded.\n\n## Known Facts\n\nNot recorded.\n\n## Current Plan\n\nNot recorded.\n\n## Next Step\n\nNot recorded.\n"
	if got := store.WorkingMemoryPath(); got != filepath.Join(sessionDir, "working_memory.md") {
		t.Fatalf("WorkingMemoryPath() = %q", got)
	}
	if got, err := store.LoadWorkingMemory(); err != nil || got != wantInitial {
		t.Fatalf("LoadWorkingMemory() = %q, %v; want initial template", got, err)
	}
	if err := store.ReplaceWorkingMemory("\nupdated state\n"); err != nil {
		t.Fatalf("ReplaceWorkingMemory() error = %v", err)
	}
	if got, err := store.LoadWorkingMemory(); err != nil || got != "updated state\n" {
		t.Fatalf("LoadWorkingMemory() after replace = %q, %v", got, err)
	}
	if err := store.AppendWorkingMemory(""); err != nil {
		t.Fatalf("AppendWorkingMemory(empty) error = %v", err)
	}
	store.now = func() time.Time { return time.Date(2026, 8, 13, 9, 10, 11, 0, time.UTC) }
	if err := store.AppendWorkingMemory("\nnew fact\n"); err != nil {
		t.Fatalf("AppendWorkingMemory(note) error = %v", err)
	}
	if got, err := store.LoadWorkingMemory(); err != nil || got != "updated state\n\n## Note 2026-08-13T09:10:11Z\n\nnew fact\n" {
		t.Fatalf("LoadWorkingMemory() after append = %q, %v", got, err)
	}
}

func TestProjectStoreDoesNotCreateSessionWorkingMemory(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	if got := store.WorkingMemoryPath(); got != "" {
		t.Fatalf("project WorkingMemoryPath() = %q, want empty", got)
	}
	if _, err := os.Stat(filepath.Join(workDir, "working_memory.md")); !os.IsNotExist(err) {
		t.Fatalf("project working_memory.md must not be created: %v", err)
	}
}

func TestEnsureWorkingMemoryPreservesExistingContent(t *testing.T) {
	sessionDir := t.TempDir()
	path := filepath.Join(sessionDir, "working_memory.md")
	if err := os.WriteFile(path, []byte("existing bytes\n"), 0600); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}
	store := NewSessionStore(t.TempDir(), sessionDir)
	if err := store.EnsureWorkingMemory(); err != nil {
		t.Fatalf("EnsureWorkingMemory() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(existing) error = %v", err)
	}
	if string(data) != "existing bytes\n" {
		t.Fatalf("existing working memory = %q", data)
	}
}

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
