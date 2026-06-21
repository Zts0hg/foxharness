package memory

import (
	"os"
	"path/filepath"
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
