package checkpoint

import (
	"bufio"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
)

const (
	snapshotAction       = "snapshot"
	snapshotUpdateAction = "snapshot_update"
)

// SnapshotRecord is one persisted checkpoint log entry.
type SnapshotRecord struct {
	Seq      int64               `json:"seq"`
	Action   string              `json:"action"`
	Snapshot FileHistorySnapshot `json:"snapshot"`
}

// RecordSnapshot persists a snapshot to the session checkpoint log.
func (c *FileCheckpointer) RecordSnapshot(messageID string, snapshot FileHistorySnapshot, isUpdate bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if snapshot.MessageID == "" {
		snapshot.MessageID = messageID
	}
	return c.recordSnapshotLocked(snapshot, isUpdate)
}

// RestoreStateFromLog rebuilds in-memory checkpoint state from checkpoints.jsonl.
func (c *FileCheckpointer) RestoreStateFromLog() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.state = FileHistoryState{TrackedFiles: make(map[string]bool)}
	c.recordSeq = 0

	f, err := os.Open(c.logPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record SnapshotRecord
		if err := json.Unmarshal(line, &record); err != nil {
			log.Printf("[Checkpoint] skipping corrupt snapshot log entry: %v", err)
			continue
		}
		c.replayRecordLocked(record)
		if record.Seq >= c.recordSeq {
			c.recordSeq = record.Seq + 1
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	c.rebuildTrackedFilesLocked()
	return nil
}

func (c *FileCheckpointer) recordSnapshotLocked(snapshot FileHistorySnapshot, isUpdate bool) error {
	if c.disabled {
		return nil
	}
	if err := c.fs.MkdirAll(filepath.Dir(c.logPath), 0755); err != nil {
		return err
	}
	action := snapshotAction
	if isUpdate {
		action = snapshotUpdateAction
	}
	record := SnapshotRecord{
		Seq:      c.recordSeq,
		Action:   action,
		Snapshot: cloneSnapshot(snapshot),
	}
	line, err := json.Marshal(record)
	if err != nil {
		return err
	}

	existing, err := c.fs.ReadFile(c.logPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	next := append([]byte(nil), existing...)
	next = append(next, line...)
	next = append(next, '\n')

	tmp := c.logPath + ".tmp"
	if err := c.fs.WriteFile(tmp, next, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.logPath); err != nil {
		return err
	}
	c.recordSeq++
	return nil
}

func (c *FileCheckpointer) replayRecordLocked(record SnapshotRecord) {
	snapshot := cloneSnapshot(record.Snapshot)
	if snapshot.TrackedFileBackups == nil {
		snapshot.TrackedFileBackups = make(map[string]FileHistoryBackup)
	}
	for i := range c.state.Snapshots {
		if c.state.Snapshots[i].MessageID == snapshot.MessageID {
			c.state.Snapshots[i] = snapshot
			return
		}
	}
	c.state.Snapshots = append(c.state.Snapshots, snapshot)
	if len(c.state.Snapshots) > c.maxSnapshots {
		c.state.Snapshots = append([]FileHistorySnapshot(nil), c.state.Snapshots[len(c.state.Snapshots)-c.maxSnapshots:]...)
	}
	c.state.SnapshotSequence++
}

func (c *FileCheckpointer) rebuildTrackedFilesLocked() {
	c.state.TrackedFiles = make(map[string]bool)
	for _, snapshot := range c.state.Snapshots {
		for path := range snapshot.TrackedFileBackups {
			c.state.TrackedFiles[path] = true
		}
	}
}
