package checkpoint

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
)

// Rewind restores workspace files to the selected snapshot. It does not mutate
// checkpoint state.
func (c *FileCheckpointer) Rewind(messageID string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureStateLocked()
	snapshot, ok := c.findSnapshotLocked(messageID)
	if !ok {
		return nil, fmt.Errorf("checkpoint snapshot %s not found", messageID)
	}

	var changed []string
	for _, filePath := range c.allKnownFilesLocked() {
		backup, ok := c.backupForSnapshotLocked(snapshot, filePath)
		if !ok {
			continue
		}
		changedFile, err := c.restoreFileLocked(filePath, backup)
		if err != nil {
			log.Printf("[Checkpoint] restore skipped for %s: %v", filePath, err)
			continue
		}
		if changedFile {
			changed = append(changed, filePath)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// GetDiffStats compares current files with the selected snapshot.
func (c *FileCheckpointer) GetDiffStats(messageID string) (*DiffStats, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureStateLocked()
	snapshot, ok := c.findSnapshotLocked(messageID)
	if !ok {
		return nil, fmt.Errorf("checkpoint snapshot %s not found", messageID)
	}

	stats := &DiffStats{}
	for _, filePath := range c.allKnownFilesLocked() {
		backup, ok := c.backupForSnapshotLocked(snapshot, filePath)
		if !ok {
			continue
		}
		snapshotData, err := c.readBackupDataLocked(backup)
		if err != nil {
			log.Printf("[Checkpoint] diff skipped for %s: %v", filePath, err)
			continue
		}
		currentData, err := c.fs.ReadFile(filePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				currentData = nil
			} else {
				log.Printf("[Checkpoint] diff skipped for %s: %v", filePath, err)
				continue
			}
		}
		if bytes.Equal(snapshotData, currentData) {
			continue
		}
		stats.FilesChanged++
		stats.ChangedFiles = append(stats.ChangedFiles, filePath)
		insertions, deletions := lineDiffCounts(string(snapshotData), string(currentData))
		stats.Insertions += insertions
		stats.Deletions += deletions
	}
	sort.Strings(stats.ChangedFiles)
	return stats, nil
}

// HasAnyChanges reports whether any file differs from the selected snapshot.
func (c *FileCheckpointer) HasAnyChanges(messageID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ensureStateLocked()
	snapshot, ok := c.findSnapshotLocked(messageID)
	if !ok {
		return false, fmt.Errorf("checkpoint snapshot %s not found", messageID)
	}
	for _, filePath := range c.allKnownFilesLocked() {
		backup, ok := c.backupForSnapshotLocked(snapshot, filePath)
		if !ok {
			continue
		}
		differs, err := c.fileDiffersFromBackupLocked(filePath, backup)
		if err != nil {
			log.Printf("[Checkpoint] change check skipped for %s: %v", filePath, err)
			continue
		}
		if differs {
			return true, nil
		}
	}
	return false, nil
}

func (c *FileCheckpointer) findSnapshotLocked(messageID string) (FileHistorySnapshot, bool) {
	for i := len(c.state.Snapshots) - 1; i >= 0; i-- {
		if c.state.Snapshots[i].MessageID == messageID {
			return c.state.Snapshots[i], true
		}
	}
	return FileHistorySnapshot{}, false
}

func (c *FileCheckpointer) allKnownFilesLocked() []string {
	seen := make(map[string]bool, len(c.state.TrackedFiles))
	for filePath, tracked := range c.state.TrackedFiles {
		if tracked {
			seen[filePath] = true
		}
	}
	for _, snapshot := range c.state.Snapshots {
		for filePath := range snapshot.TrackedFileBackups {
			seen[filePath] = true
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func (c *FileCheckpointer) backupForSnapshotLocked(snapshot FileHistorySnapshot, filePath string) (FileHistoryBackup, bool) {
	if backup, ok := snapshot.TrackedFileBackups[filePath]; ok {
		return backup, true
	}
	return c.earliestBackupForFileLocked(filePath)
}

func (c *FileCheckpointer) restoreFileLocked(filePath string, backup FileHistoryBackup) (bool, error) {
	differs, err := c.fileDiffersFromBackupLocked(filePath, backup)
	if err != nil {
		return false, err
	}
	if !differs {
		return false, nil
	}
	if backup.BackupFileName == "" {
		if err := c.fs.Remove(filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		return true, nil
	}

	backupPath := filepath.Join(c.checkpointsDir, backup.BackupFileName)
	info, err := c.fs.Stat(backupPath)
	if err != nil {
		return false, err
	}
	if err := c.fs.CopyFile(filePath, backupPath, info.Mode()); err != nil {
		return false, err
	}
	return true, nil
}

func (c *FileCheckpointer) fileDiffersFromBackupLocked(filePath string, backup FileHistoryBackup) (bool, error) {
	if backup.BackupFileName == "" {
		_, err := c.fs.Stat(filePath)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return err == nil, err
	}

	backupPath := filepath.Join(c.checkpointsDir, backup.BackupFileName)
	backupInfo, err := c.fs.Stat(backupPath)
	if err != nil {
		return false, err
	}
	currentInfo, err := c.fs.Stat(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	if currentInfo.Mode() != backupInfo.Mode() || currentInfo.Size() != backupInfo.Size() {
		return true, nil
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

func (c *FileCheckpointer) readBackupDataLocked(backup FileHistoryBackup) ([]byte, error) {
	if backup.BackupFileName == "" {
		return nil, nil
	}
	return c.fs.ReadFile(filepath.Join(c.checkpointsDir, backup.BackupFileName))
}

func lineDiffCounts(oldText, newText string) (int, int) {
	dmp := diffmatchpatch.New()
	oldChars, newChars, lineArray := dmp.DiffLinesToChars(oldText, newText)
	diffs := dmp.DiffMain(oldChars, newChars, false)
	diffs = dmp.DiffCharsToLines(diffs, lineArray)

	var insertions int
	var deletions int
	for _, diff := range diffs {
		switch diff.Type {
		case diffmatchpatch.DiffInsert:
			insertions += diffLineCount(diff.Text)
		case diffmatchpatch.DiffDelete:
			deletions += diffLineCount(diff.Text)
		}
	}
	return insertions, deletions
}

func diffLineCount(text string) int {
	if text == "" {
		return 0
	}
	lines := strings.Count(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		lines++
	}
	return lines
}
