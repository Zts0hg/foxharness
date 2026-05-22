package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDisabledCheckpointing(t *testing.T) {
	cp := newTestCheckpointer(t)
	cp.SetDisabled(true)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "main.go", []byte("one"), 0644)

	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if len(cp.State().Snapshots) != 0 {
		t.Fatalf("snapshots created while disabled: %#v", cp.State().Snapshots)
	}
	if _, err := os.Stat(cp.checkpointsDir); !os.IsNotExist(err) {
		t.Fatalf("checkpoints dir exists while disabled; stat err = %v", err)
	}
}

func TestCrossSessionPersistence(t *testing.T) {
	sessionDir := t.TempDir()
	workDir := t.TempDir()
	filePath := writeCheckpointTestFile(t, workDir, "main.go", []byte("one"), 0644)

	first := New(Config{SessionDir: sessionDir}).(*FileCheckpointer)
	if err := first.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if err := first.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("two"), 0644); err != nil {
		t.Fatalf("WriteFile(two) error = %v", err)
	}
	if err := first.MakeSnapshot("2"); err != nil {
		t.Fatalf("MakeSnapshot(2) error = %v", err)
	}

	second := New(Config{SessionDir: sessionDir}).(*FileCheckpointer)
	if err := second.RestoreStateFromLog(); err != nil {
		t.Fatalf("RestoreStateFromLog() error = %v", err)
	}
	if len(second.State().Snapshots) != 2 {
		t.Fatalf("restored snapshots = %#v, want 2", second.State().Snapshots)
	}
	if err := os.WriteFile(filePath, []byte("three"), 0644); err != nil {
		t.Fatalf("WriteFile(three) error = %v", err)
	}
	if _, err := second.Rewind("1"); err != nil {
		t.Fatalf("Rewind(1) error = %v", err)
	}
	if string(readCheckpointTestFile(t, filePath)) != "one" {
		t.Fatalf("cross-session rewind did not restore original content")
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "checkpoints")); err != nil {
		t.Fatalf("backup dir missing after resume: %v", err)
	}
}
