package checkpoint

import (
	"path/filepath"
	"sync"
)

const defaultMaxSnapshots = 100

// Checkpointer provides checkpoint and rewind functionality for a session.
type Checkpointer interface {
	TrackEdit(filePath, messageID string) error
	MakeSnapshot(messageID string) error
	Rewind(messageID string) ([]string, error)
	GetDiffStats(messageID string) (*DiffStats, error)
	HasAnyChanges(messageID string) (bool, error)
	SetDisabled(disabled bool)
	IsDisabled() bool
	RestoreStateFromLog() error
}

// Config holds checkpoint configuration.
type Config struct {
	SessionDir   string
	MaxSnapshots int
	FS           FS
}

// FileCheckpointer is the default filesystem-backed Checkpointer.
type FileCheckpointer struct {
	mu sync.Mutex

	sessionDir     string
	checkpointsDir string
	logPath        string
	maxSnapshots   int
	fs             FS

	state     FileHistoryState
	recordSeq int64
	disabled  bool
}

// New creates a filesystem-backed checkpointer for a session.
func New(cfg Config) Checkpointer {
	if cfg.SessionDir == "" {
		cfg.SessionDir = "."
	}
	if cfg.MaxSnapshots <= 0 {
		cfg.MaxSnapshots = defaultMaxSnapshots
	}
	if cfg.FS == nil {
		cfg.FS = osFS{}
	}
	cp := &FileCheckpointer{
		sessionDir:     cfg.SessionDir,
		checkpointsDir: filepath.Join(cfg.SessionDir, "checkpoints"),
		logPath:        filepath.Join(cfg.SessionDir, "checkpoints.jsonl"),
		maxSnapshots:   cfg.MaxSnapshots,
		fs:             cfg.FS,
		state: FileHistoryState{
			TrackedFiles: make(map[string]bool),
		},
	}
	return cp
}

// SetDisabled turns checkpoint operations on or off.
func (c *FileCheckpointer) SetDisabled(disabled bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.disabled = disabled
}

// IsDisabled reports whether checkpoint operations are disabled.
func (c *FileCheckpointer) IsDisabled() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disabled
}

// State returns a copy of the current in-memory checkpoint state.
func (c *FileCheckpointer) State() FileHistoryState {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureStateLocked()
	return cloneState(c.state)
}

func (c *FileCheckpointer) ensureStateLocked() {
	if c.state.TrackedFiles == nil {
		c.state.TrackedFiles = make(map[string]bool)
	}
	for i := range c.state.Snapshots {
		if c.state.Snapshots[i].TrackedFileBackups == nil {
			c.state.Snapshots[i].TrackedFileBackups = make(map[string]FileHistoryBackup)
		}
	}
}

func cloneState(state FileHistoryState) FileHistoryState {
	cloned := FileHistoryState{
		Snapshots:        make([]FileHistorySnapshot, len(state.Snapshots)),
		TrackedFiles:     make(map[string]bool, len(state.TrackedFiles)),
		SnapshotSequence: state.SnapshotSequence,
	}
	for path, tracked := range state.TrackedFiles {
		cloned.TrackedFiles[path] = tracked
	}
	for i, snapshot := range state.Snapshots {
		cloned.Snapshots[i] = cloneSnapshot(snapshot)
	}
	return cloned
}

func cloneSnapshot(snapshot FileHistorySnapshot) FileHistorySnapshot {
	cloned := FileHistorySnapshot{
		MessageID:          snapshot.MessageID,
		TrackedFileBackups: make(map[string]FileHistoryBackup, len(snapshot.TrackedFileBackups)),
		Timestamp:          snapshot.Timestamp,
	}
	for path, backup := range snapshot.TrackedFileBackups {
		cloned.TrackedFileBackups[path] = backup
	}
	return cloned
}
