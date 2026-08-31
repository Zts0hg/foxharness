package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StoredRun contains persisted metadata for one submitted task or message.
type StoredRun struct {
	ID        RunID      `json:"id"`
	SessionID ID         `json:"session_id"`
	RootDir   string     `json:"root_dir"`
	Prompt    string     `json:"prompt"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// StartRun persists a new stored run for the provided session.
func (s *FileStore) StartRun(storedSession *StoredSession, prompt string) (*StoredRun, error) {
	return startStoredRun(storedSession, prompt)
}

func startStoredRun(storedSession *StoredSession, prompt string) (*StoredRun, error) {
	id := newRunID()
	root := filepath.Join(storedSession.RunsDir(), string(id))
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0755); err != nil {
		return nil, fmt.Errorf("创建 Run 目录失败: %w", err)
	}

	r := &StoredRun{
		ID:        id,
		SessionID: storedSession.ID,
		RootDir:   root,
		Prompt:    prompt,
		StartedAt: time.Now(),
	}
	if err := r.write(); err != nil {
		return nil, err
	}
	return r, nil
}

// FinishRun marks a stored run as completed and rewrites its metadata.
func (s *FileStore) FinishRun(run *StoredRun) error {
	return finishStoredRun(run)
}

func finishStoredRun(r *StoredRun) error {
	now := time.Now()
	r.EndedAt = &now
	return r.write()
}

// MetricsPath returns the run-local metrics path.
func (r *StoredRun) MetricsPath() string {
	return filepath.Join(r.RootDir, "metrics.jsonl")
}

// TracePath returns the run-local trace path.
func (r *StoredRun) TracePath() string {
	return filepath.Join(r.RootDir, "trace.jsonl")
}

// ArtifactsDir returns the run-local artifacts directory.
func (r *StoredRun) ArtifactsDir() string {
	return filepath.Join(r.RootDir, "artifacts")
}

func (r *StoredRun) write() error {
	if err := writeJSON(filepath.Join(r.RootDir, "run.json"), r); err != nil {
		return fmt.Errorf("写入 Run 元数据失败: %w", err)
	}
	return nil
}
