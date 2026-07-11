package memory

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStateHistorySnapshotIsFirstWriteWins(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(t.TempDir(), dir)
	writeStateTestFile(t, dir, "PLAN.md", "old plan")
	writeStateTestFile(t, dir, "TODO.md", "old todo")

	history := NewStateHistory(store)
	if err := history.SnapshotBeforeMessage(7); err != nil {
		t.Fatalf("SnapshotBeforeMessage() error = %v", err)
	}
	writeStateTestFile(t, dir, "PLAN.md", "new plan")
	writeStateTestFile(t, dir, "TODO.md", "new todo")
	if err := history.SnapshotBeforeMessage(7); err != nil {
		t.Fatalf("second SnapshotBeforeMessage() error = %v", err)
	}
	if err := history.RestoreBeforeMessage(7); err != nil {
		t.Fatalf("RestoreBeforeMessage() error = %v", err)
	}

	if got := readStateTestFile(t, store.PlanPath()); got != "old plan" {
		t.Fatalf("PLAN.md = %q, want old plan", got)
	}
	if got := readStateTestFile(t, store.TodoPath()); got != "old todo" {
		t.Fatalf("TODO.md = %q, want old todo", got)
	}
}

func TestStateHistoryRestoresMissingFiles(t *testing.T) {
	dir := t.TempDir()
	store := NewSessionStore(t.TempDir(), dir)
	history := NewStateHistory(store)

	if err := history.SnapshotBeforeMessage(3); err != nil {
		t.Fatalf("SnapshotBeforeMessage() error = %v", err)
	}
	writeStateTestFile(t, dir, "PLAN.md", "future plan")
	writeStateTestFile(t, dir, "TODO.md", "future todo")

	if err := history.RestoreBeforeMessage(3); err != nil {
		t.Fatalf("RestoreBeforeMessage() error = %v", err)
	}
	for _, path := range []string{store.PlanPath(), store.TodoPath()} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("%s should not exist after restore, stat err = %v", filepath.Base(path), err)
		}
	}
}

func writeStateTestFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readStateTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestStateHistoryMissingSnapshot(t *testing.T) {
	history := NewStateHistory(NewSessionStore(t.TempDir(), t.TempDir()))
	err := history.RestoreBeforeMessage(99)
	if !errors.Is(err, ErrStateSnapshotNotFound) {
		t.Fatalf("RestoreBeforeMessage() error = %v, want ErrStateSnapshotNotFound", err)
	}
}
