package checkpoint

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
)

func TestCreateBackup(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "main.go", []byte("before\n"), 0644)

	cp.mu.Lock()
	backup, err := cp.createBackupLocked(filePath, 1)
	cp.mu.Unlock()
	if err != nil {
		t.Fatalf("createBackupLocked() error = %v", err)
	}

	got := readCheckpointTestFile(t, filepath.Join(cp.checkpointsDir, backup.BackupFileName))
	if string(got) != "before\n" {
		t.Fatalf("backup content = %q, want before", got)
	}
}

func TestBackupFileName(t *testing.T) {
	got := backupFileName("/tmp/project/main.go", 12)
	if !regexp.MustCompile(`^[0-9a-f]{16}@v12$`).MatchString(got) {
		t.Fatalf("backupFileName() = %q, want {16hex}@v12", got)
	}
}

func TestTrackEdit(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "main.go", []byte("before"), 0644)

	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}

	state := cp.State()
	backup := state.Snapshots[0].TrackedFileBackups[filePath]
	if backup.BackupFileName == "" || backup.Version != 1 {
		t.Fatalf("backup = %#v, want v1 backup", backup)
	}
	if string(readCheckpointTestFile(t, filepath.Join(cp.checkpointsDir, backup.BackupFileName))) != "before" {
		t.Fatalf("backup did not capture pre-edit content")
	}
}

func TestTrackEditNullBackup(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := filepath.Join(t.TempDir(), "new.go")

	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}

	backup := cp.State().Snapshots[0].TrackedFileBackups[filePath]
	if backup.BackupFileName != "" || backup.Version != 1 {
		t.Fatalf("null backup = %#v, want empty filename v1", backup)
	}
}

func TestBackupPreservesPermissions(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "tool.sh", []byte("#!/bin/sh\n"), 0755)

	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}
	backup := cp.State().Snapshots[0].TrackedFileBackups[filePath]
	info, err := os.Stat(filepath.Join(cp.checkpointsDir, backup.BackupFileName))
	if err != nil {
		t.Fatalf("Stat(backup) error = %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("backup mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestBackupEdgeCases(t *testing.T) {
	cp := newTestCheckpointer(t)
	dir := t.TempDir()
	large := bytes.Repeat([]byte("x"), 2*1024*1024)
	cases := []struct {
		name string
		data []byte
	}{
		{name: "large.bin", data: large},
		{name: "binary.bin", data: []byte{0x00, 0xff, 0x01, 0x02}},
		{name: "empty.txt", data: nil},
	}
	for _, tc := range cases {
		filePath := writeCheckpointTestFile(t, dir, tc.name, tc.data, 0644)
		if err := cp.TrackEdit(filePath, tc.name); err != nil {
			t.Fatalf("TrackEdit(%s) error = %v", tc.name, err)
		}
		backup := cp.State().Snapshots[len(cp.State().Snapshots)-1].TrackedFileBackups[filePath]
		got := readCheckpointTestFile(t, filepath.Join(cp.checkpointsDir, backup.BackupFileName))
		if !bytes.Equal(got, tc.data) {
			t.Fatalf("backup %s content mismatch", tc.name)
		}
	}
}

func TestSymlinkBackup(t *testing.T) {
	cp := newTestCheckpointer(t)
	dir := t.TempDir()
	target := writeCheckpointTestFile(t, dir, "target.txt", []byte("target"), 0644)
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}

	if err := cp.TrackEdit(link, "1"); err != nil {
		t.Fatalf("TrackEdit(symlink) error = %v", err)
	}
	backup := cp.State().Snapshots[0].TrackedFileBackups[link]
	if string(readCheckpointTestFile(t, filepath.Join(cp.checkpointsDir, backup.BackupFileName))) != "target" {
		t.Fatalf("symlink backup did not follow target")
	}
}

func TestConcurrentTrackEdit(t *testing.T) {
	cp := newTestCheckpointer(t)
	dir := t.TempDir()

	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			filePath := writeCheckpointTestFile(t, dir, filepath.Join("files", string(rune('a'+i))+".txt"), []byte{byte(i)}, 0644)
			if err := cp.TrackEdit(filePath, "1"); err != nil {
				t.Errorf("TrackEdit(%s) error = %v", filePath, err)
			}
		}()
	}
	wg.Wait()

	state := cp.State()
	if len(state.TrackedFiles) != 12 {
		t.Fatalf("tracked files = %d, want 12", len(state.TrackedFiles))
	}
}

func newTestCheckpointer(t *testing.T) *FileCheckpointer {
	t.Helper()
	cp, ok := New(Config{SessionDir: t.TempDir()}).(*FileCheckpointer)
	if !ok {
		t.Fatalf("New() returned %T, want *FileCheckpointer", cp)
	}
	return cp
}

func writeCheckpointTestFile(t *testing.T, dir, name string, data []byte, perm os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	return path
}

func readCheckpointTestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}
