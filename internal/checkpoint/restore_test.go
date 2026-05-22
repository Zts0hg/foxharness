package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRewind(t *testing.T) {
	cp := newTestCheckpointer(t)
	dir := t.TempDir()
	filePath := writeCheckpointTestFile(t, dir, "main.go", []byte("one\n"), 0644)
	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot(1) error = %v", err)
	}
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("two\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	changed, err := cp.Rewind("1")
	if err != nil {
		t.Fatalf("Rewind() error = %v", err)
	}
	if len(changed) != 1 || changed[0] != filePath {
		t.Fatalf("changed files = %#v, want %s", changed, filePath)
	}
	if string(readCheckpointTestFile(t, filePath)) != "one\n" {
		t.Fatalf("file was not restored")
	}
}

func TestRewindNullBackup(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := filepath.Join(t.TempDir(), "new.go")
	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("created"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := cp.Rewind("1"); err != nil {
		t.Fatalf("Rewind() error = %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("new file still exists; stat err = %v", err)
	}
}

func TestGetDiffStats(t *testing.T) {
	cp := newTestCheckpointer(t)
	dir := t.TempDir()
	a := writeCheckpointTestFile(t, dir, "a.txt", []byte("one\ntwo\n"), 0644)
	b := writeCheckpointTestFile(t, dir, "b.txt", []byte("old\n"), 0644)
	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if err := cp.TrackEdit(a, "1"); err != nil {
		t.Fatalf("TrackEdit(a) error = %v", err)
	}
	if err := cp.TrackEdit(b, "1"); err != nil {
		t.Fatalf("TrackEdit(b) error = %v", err)
	}
	if err := os.WriteFile(a, []byte("one\ntwo\nthree\n"), 0644); err != nil {
		t.Fatalf("WriteFile(a) error = %v", err)
	}
	if err := os.WriteFile(b, []byte("new\n"), 0644); err != nil {
		t.Fatalf("WriteFile(b) error = %v", err)
	}

	stats, err := cp.GetDiffStats("1")
	if err != nil {
		t.Fatalf("GetDiffStats() error = %v", err)
	}
	if stats.FilesChanged != 2 || stats.Insertions != 2 || stats.Deletions != 1 {
		t.Fatalf("stats = %#v, want 2 files +2 -1", stats)
	}
}

func TestHasAnyChangesAndCombinedRestorePurity(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "main.go", []byte("one"), 0644)
	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	before := cp.State()

	hasChanges, err := cp.HasAnyChanges("1")
	if err != nil {
		t.Fatalf("HasAnyChanges() error = %v", err)
	}
	if hasChanges {
		t.Fatalf("HasAnyChanges() = true before modification")
	}
	if err := os.WriteFile(filePath, []byte("two"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	hasChanges, err = cp.HasAnyChanges("1")
	if err != nil {
		t.Fatalf("HasAnyChanges(after) error = %v", err)
	}
	if !hasChanges {
		t.Fatalf("HasAnyChanges() = false after modification")
	}
	if _, err := cp.Rewind("1"); err != nil {
		t.Fatalf("Rewind() error = %v", err)
	}
	after := cp.State()
	if len(before.Snapshots) != len(after.Snapshots) || before.Snapshots[0].TrackedFileBackups[filePath] != after.Snapshots[0].TrackedFileBackups[filePath] {
		t.Fatalf("Rewind mutated state: before=%#v after=%#v", before, after)
	}
}

func TestMissingBackupOnRestore(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "main.go", []byte("one"), 0644)
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	backup := cp.State().Snapshots[0].TrackedFileBackups[filePath]
	if err := os.Remove(filepath.Join(cp.checkpointsDir, backup.BackupFileName)); err != nil {
		t.Fatalf("Remove(backup) error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("two"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	changed, err := cp.Rewind("1")
	if err != nil {
		t.Fatalf("Rewind() error = %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed files = %#v, want none when backup missing", changed)
	}
}
