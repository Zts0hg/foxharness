package checkpoint

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecordSnapshot(t *testing.T) {
	cp := newTestCheckpointer(t)
	snapshot := FileHistorySnapshot{
		MessageID:          "1",
		TrackedFileBackups: map[string]FileHistoryBackup{},
	}
	if err := cp.RecordSnapshot("1", snapshot, false); err != nil {
		t.Fatalf("RecordSnapshot() error = %v", err)
	}
	data := readCheckpointTestFile(t, cp.logPath)
	var record SnapshotRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &record); err != nil {
		t.Fatalf("snapshot log is not valid JSON: %v", err)
	}
	if record.Action != snapshotAction || record.Snapshot.MessageID != "1" {
		t.Fatalf("record = %#v, want snapshot for 1", record)
	}
}

func TestRestoreStateFromLog(t *testing.T) {
	cp := newTestCheckpointer(t)
	filePath := writeCheckpointTestFile(t, t.TempDir(), "main.go", []byte("one"), 0644)
	if err := cp.MakeSnapshot("1"); err != nil {
		t.Fatalf("MakeSnapshot() error = %v", err)
	}
	if err := cp.TrackEdit(filePath, "1"); err != nil {
		t.Fatalf("TrackEdit() error = %v", err)
	}

	next, ok := New(Config{SessionDir: cp.sessionDir}).(*FileCheckpointer)
	if !ok {
		t.Fatalf("New() returned unexpected type")
	}
	if err := next.RestoreStateFromLog(); err != nil {
		t.Fatalf("RestoreStateFromLog() error = %v", err)
	}
	state := next.State()
	if len(state.Snapshots) != 1 || state.Snapshots[0].TrackedFileBackups[filePath].Version != 1 {
		t.Fatalf("restored state = %#v", state)
	}
}

func TestAtomicPersistence(t *testing.T) {
	cp := newTestCheckpointer(t)
	if err := cp.RecordSnapshot("1", FileHistorySnapshot{MessageID: "1"}, false); err != nil {
		t.Fatalf("RecordSnapshot() error = %v", err)
	}
	if _, err := os.Stat(cp.logPath + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp log remains after atomic write; stat err = %v", err)
	}
	if _, err := os.Stat(cp.logPath); err != nil {
		t.Fatalf("checkpoint log missing: %v", err)
	}
}

func TestCorruptSnapshotLog(t *testing.T) {
	cp := newTestCheckpointer(t)
	valid := SnapshotRecord{
		Seq:    1,
		Action: snapshotAction,
		Snapshot: FileHistorySnapshot{
			MessageID:          "ok",
			TrackedFileBackups: map[string]FileHistoryBackup{},
		},
	}
	line, err := json.Marshal(valid)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(cp.logPath, []byte("{bad json\n"+string(line)+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile(log) error = %v", err)
	}
	if err := cp.RestoreStateFromLog(); err != nil {
		t.Fatalf("RestoreStateFromLog() error = %v", err)
	}
	if len(cp.State().Snapshots) != 1 || cp.State().Snapshots[0].MessageID != "ok" {
		t.Fatalf("state after corrupt log = %#v", cp.State())
	}
}

func TestCheckpointLogIsJSONL(t *testing.T) {
	cp := newTestCheckpointer(t)
	for _, id := range []string{"1", "2"} {
		if err := cp.RecordSnapshot(id, FileHistorySnapshot{MessageID: id}, false); err != nil {
			t.Fatalf("RecordSnapshot(%s) error = %v", id, err)
		}
	}
	file, err := os.Open(filepath.Clean(cp.logPath))
	if err != nil {
		t.Fatalf("Open(log) error = %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	lines := 0
	for scanner.Scan() {
		lines++
		var record SnapshotRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("line %d invalid JSON: %v", lines, err)
		}
	}
	if lines != 2 {
		t.Fatalf("log lines = %d, want 2", lines)
	}
}
