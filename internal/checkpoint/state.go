package checkpoint

import "time"

// FileHistoryState is the in-memory checkpoint history for one session.
type FileHistoryState struct {
	Snapshots        []FileHistorySnapshot `json:"snapshots"`
	TrackedFiles     map[string]bool       `json:"tracked_files"`
	SnapshotSequence int                   `json:"snapshot_sequence"`
}

// FileHistorySnapshot records one rewind target associated with a user
// message.
type FileHistorySnapshot struct {
	MessageID          string                       `json:"message_id"`
	TrackedFileBackups map[string]FileHistoryBackup `json:"tracked_file_backups"`
	Timestamp          time.Time                    `json:"timestamp"`
}

// FileHistoryBackup points at one stored file version. An empty backup file
// name is the canonical null backup marker for a file that did not exist.
type FileHistoryBackup struct {
	BackupFileName string    `json:"backup_file_name"`
	Version        int       `json:"version"`
	BackupTime     time.Time `json:"backup_time"`
}

// DiffStats summarizes the difference between the current workspace and a
// checkpoint snapshot.
type DiffStats struct {
	FilesChanged int      `json:"files_changed"`
	Insertions   int      `json:"insertions"`
	Deletions    int      `json:"deletions"`
	ChangedFiles []string `json:"changed_files"`
}

// SelectableMessage is a user-authored message that can be used as a rewind
// target in the TUI.
type SelectableMessage struct {
	Seq       int64     `json:"seq"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	IsCurrent bool      `json:"is_current"`
}
