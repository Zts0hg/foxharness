package checkpoint

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestChangeDetection(t *testing.T) {
	cp := newTestCheckpointer(t)
	dir := t.TempDir()
	filePath := writeCheckpointTestFile(t, dir, "main.go", []byte("same"), 0644)
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	backup := cp.State().Snapshots[0].TrackedFileBackups[filePath]

	cp.mu.Lock()
	changed, err := cp.fileChangedLocked(filePath, backup)
	cp.mu.Unlock()
	if err != nil || changed {
		t.Fatalf("unchanged file changed=%v err=%v, want false nil", changed, err)
	}

	if err := os.WriteFile(filePath, []byte("different-size"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cp.mu.Lock()
	changed, err = cp.fileChangedLocked(filePath, backup)
	cp.mu.Unlock()
	if err != nil || !changed {
		t.Fatalf("size change changed=%v err=%v, want true nil", changed, err)
	}

	if err := os.WriteFile(filePath, []byte("same"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(filePath, 0600); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	cp.mu.Lock()
	changed, err = cp.fileChangedLocked(filePath, backup)
	cp.mu.Unlock()
	if err != nil || !changed {
		t.Fatalf("mode change changed=%v err=%v, want true nil", changed, err)
	}

	if err := os.Chmod(filePath, 0644); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("SAME"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	future := backup.BackupTime.Add(time.Second)
	if err := os.Chtimes(filePath, future, future); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}
	cp.mu.Lock()
	changed, err = cp.fileChangedLocked(filePath, backup)
	cp.mu.Unlock()
	if err != nil || !changed {
		t.Fatalf("content change changed=%v err=%v, want true nil", changed, err)
	}

	past := backup.BackupTime.Add(-time.Second)
	if err := os.Chtimes(filePath, past, past); err != nil {
		t.Fatalf("Chtimes(past) error = %v", err)
	}
	cp.mu.Lock()
	changed, err = cp.fileChangedLocked(filePath, backup)
	cp.mu.Unlock()
	if err != nil || changed {
		t.Fatalf("mtime fast path changed=%v err=%v, want false nil", changed, err)
	}

	if err := os.Remove(filePath); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	cp.mu.Lock()
	changed, err = cp.fileChangedLocked(filePath, backup)
	cp.mu.Unlock()
	if err != nil || !changed {
		t.Fatalf("missing current changed=%v err=%v, want true nil", changed, err)
	}
}

func TestMakeSnapshot(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "main.go", []byte("v1"), 0644)

	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot(1) error = %v", err)
	}
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	if err := os.WriteFile(filePath, []byte("v2"), 0644); err != nil {
		t.Fatalf("WriteFile(v2) error = %v", err)
	}
	if err := cp.MakeSnapshot("2"); err != nil {
		t.Fatalf("MakeSnapshot(2) error = %v", err)
	}

	state := cp.State()
	if len(state.Snapshots) != 2 {
		t.Fatalf("snapshots len = %d, want 2", len(state.Snapshots))
	}
	if state.Snapshots[0].MessageID != "1" || state.Snapshots[1].MessageID != "2" {
		t.Fatalf("message IDs = %#v", state.Snapshots)
	}
	if state.Snapshots[0].TrackedFileBackups[filePath].Version != 1 ||
		state.Snapshots[1].TrackedFileBackups[filePath].Version != 2 {
		t.Fatalf("snapshot versions = %#v", state.Snapshots)
	}
}

func TestSnapshotFIFOEviction(t *testing.T) {
	cp := newTestCheckpointer(t)
	cp.maxSnapshots = 100
	for i := 0; i < 101; i++ {
		if err := cp.MakeSnapshot(filepath.Base(time.Unix(int64(i), 0).String())); err != nil {
			t.Fatalf("MakeSnapshot(%d) error = %v", i, err)
		}
	}
	state := cp.State()
	if len(state.Snapshots) != 100 {
		t.Fatalf("snapshots len = %d, want 100", len(state.Snapshots))
	}
}

func TestSnapshotEdgeCases(t *testing.T) {
	cp := newTestCheckpointer(t)
	if err := os.RemoveAll(cp.checkpointsDir); err != nil {
		t.Fatalf("RemoveAll() error = %v", err)
	}
	if err := cp.MakeSnapshot("empty"); err != nil {
		t.Fatalf("MakeSnapshot(empty) error = %v", err)
	}
	if _, err := os.Stat(cp.checkpointsDir); err != nil {
		t.Fatalf("checkpoints dir was not recreated: %v", err)
	}
	if got := len(cp.State().Snapshots[0].TrackedFileBackups); got != 0 {
		t.Fatalf("empty snapshot backups = %d, want 0", got)
	}
}
