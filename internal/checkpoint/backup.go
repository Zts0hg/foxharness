package checkpoint

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// TrackEdit records a file's current state before a write-capable tool edits
// it.
func (c *FileCheckpointer) TrackEdit(filePath, messageID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.disabled {
		return nil
	}
	c.ensureStateLocked()
	if err := c.fs.MkdirAll(c.checkpointsDir, 0755); err != nil {
		return err
	}

	snapshot := c.ensureLatestSnapshotLocked(messageID)
	if _, ok := snapshot.TrackedFileBackups[filePath]; ok {
		return nil
	}

	backup, err := c.createBackupLocked(filePath, 1)
	if err != nil {
		return fmt.Errorf("create backup for %s: %w", filePath, err)
	}
	c.state.TrackedFiles[filePath] = true
	snapshot.TrackedFileBackups[filePath] = backup
	return c.recordSnapshotLocked(*snapshot, true)
}

func (c *FileCheckpointer) ensureLatestSnapshotLocked(messageID string) *FileHistorySnapshot {
	if messageID == "" {
		if len(c.state.Snapshots) > 0 {
			return &c.state.Snapshots[len(c.state.Snapshots)-1]
		}
		messageID = strconv.Itoa(c.state.SnapshotSequence)
	}
	if len(c.state.Snapshots) > 0 {
		latest := &c.state.Snapshots[len(c.state.Snapshots)-1]
		if latest.MessageID == messageID {
			if latest.TrackedFileBackups == nil {
				latest.TrackedFileBackups = make(map[string]FileHistoryBackup)
			}
			return latest
		}
	}

	snapshot := FileHistorySnapshot{
		MessageID:          messageID,
		TrackedFileBackups: make(map[string]FileHistoryBackup),
		Timestamp:          time.Now(),
	}
	c.appendSnapshotLocked(snapshot)
	return &c.state.Snapshots[len(c.state.Snapshots)-1]
}

func (c *FileCheckpointer) appendSnapshotLocked(snapshot FileHistorySnapshot) {
	if snapshot.TrackedFileBackups == nil {
		snapshot.TrackedFileBackups = make(map[string]FileHistoryBackup)
	}
	c.state.Snapshots = append(c.state.Snapshots, snapshot)
	if len(c.state.Snapshots) > c.maxSnapshots {
		c.state.Snapshots = append([]FileHistorySnapshot(nil), c.state.Snapshots[len(c.state.Snapshots)-c.maxSnapshots:]...)
	}
	c.state.SnapshotSequence++
}

func (c *FileCheckpointer) createBackupLocked(filePath string, version int) (FileHistoryBackup, error) {
	info, err := c.fs.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileHistoryBackup{
				BackupFileName: "",
				Version:        version,
				BackupTime:     time.Now(),
			}, nil
		}
		return FileHistoryBackup{}, err
	}
	if info.IsDir() {
		return FileHistoryBackup{}, fmt.Errorf("cannot back up directory: %s", filePath)
	}
	if err := c.fs.MkdirAll(c.checkpointsDir, 0755); err != nil {
		return FileHistoryBackup{}, err
	}
	name := backupFileName(filePath, version)
	if err := c.fs.CopyFile(filepath.Join(c.checkpointsDir, name), filePath, info.Mode()); err != nil {
		return FileHistoryBackup{}, err
	}
	return FileHistoryBackup{
		BackupFileName: name,
		Version:        version,
		BackupTime:     time.Now(),
	}, nil
}

func backupFileName(filePath string, version int) string {
	sum := sha256.Sum256([]byte(filePath))
	encoded := hex.EncodeToString(sum[:])
	return fmt.Sprintf("%s@v%d", encoded[:16], version)
}
