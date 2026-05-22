package checkpoint

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// MakeSnapshot creates a rewind target for a user message.
func (c *FileCheckpointer) MakeSnapshot(messageID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.disabled {
		return nil
	}
	c.ensureStateLocked()
	if len(c.state.Snapshots) > 0 && c.state.Snapshots[len(c.state.Snapshots)-1].MessageID == messageID {
		return nil
	}
	if err := c.fs.MkdirAll(c.checkpointsDir, 0755); err != nil {
		return err
	}

	snapshot := FileHistorySnapshot{
		MessageID:          messageID,
		TrackedFileBackups: make(map[string]FileHistoryBackup),
		Timestamp:          time.Now(),
	}
	for _, filePath := range c.trackedFilePathsLocked() {
		previous, ok := c.latestBackupForFileLocked(filePath)
		if !ok {
			backup, err := c.createBackupLocked(filePath, 1)
			if err != nil {
				return err
			}
			snapshot.TrackedFileBackups[filePath] = backup
			continue
		}

		changed, err := c.fileChangedLocked(filePath, previous)
		if err != nil {
			log.Printf("[Checkpoint] change detection failed for %s: %v", filePath, err)
			changed = true
		}
		if !changed {
			snapshot.TrackedFileBackups[filePath] = previous
			continue
		}

		backup, err := c.createBackupLocked(filePath, previous.Version+1)
		if err != nil {
			return err
		}
		snapshot.TrackedFileBackups[filePath] = backup
	}

	c.appendSnapshotLocked(snapshot)
	return c.recordSnapshotLocked(snapshot, false)
}

func (c *FileCheckpointer) trackedFilePathsLocked() []string {
	paths := make([]string, 0, len(c.state.TrackedFiles))
	for path, tracked := range c.state.TrackedFiles {
		if tracked {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

func (c *FileCheckpointer) latestBackupForFileLocked(filePath string) (FileHistoryBackup, bool) {
	for i := len(c.state.Snapshots) - 1; i >= 0; i-- {
		backup, ok := c.state.Snapshots[i].TrackedFileBackups[filePath]
		if ok {
			return backup, true
		}
	}
	return FileHistoryBackup{}, false
}

func (c *FileCheckpointer) earliestBackupForFileLocked(filePath string) (FileHistoryBackup, bool) {
	for i := 0; i < len(c.state.Snapshots); i++ {
		backup, ok := c.state.Snapshots[i].TrackedFileBackups[filePath]
		if ok {
			return backup, true
		}
	}
	return FileHistoryBackup{}, false
}

func (c *FileCheckpointer) fileChangedLocked(filePath string, backup FileHistoryBackup) (bool, error) {
	currentInfo, currentErr := c.fs.Stat(filePath)
	currentMissing := errors.Is(currentErr, os.ErrNotExist)
	if currentErr != nil && !currentMissing {
		return false, currentErr
	}

	if backup.BackupFileName == "" {
		return !currentMissing, nil
	}
	if currentMissing {
		return true, nil
	}

	backupPath := filepath.Join(c.checkpointsDir, backup.BackupFileName)
	backupInfo, err := c.fs.Stat(backupPath)
	if err != nil {
		return true, err
	}
	if currentInfo.Mode() != backupInfo.Mode() || currentInfo.Size() != backupInfo.Size() {
		return true, nil
	}
	if currentInfo.ModTime().Before(backup.BackupTime) {
		return false, nil
	}

	currentData, err := c.fs.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	backupData, err := c.fs.ReadFile(backupPath)
	if err != nil {
		return false, err
	}
	return !bytes.Equal(currentData, backupData), nil
}
