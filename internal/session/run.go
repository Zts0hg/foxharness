package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Run represents one user-submitted task or message within a long-lived
// session. Metrics, traces, and run-local artifacts are written under the run
// directory so multiple runs in the same session remain distinguishable.
type Run struct {
	ID        string     `json:"id"`
	SessionID string     `json:"session_id"`
	RootDir   string     `json:"root_dir"`
	Prompt    string     `json:"prompt"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// StartRun creates a run directory and writes initial run metadata.
func (s *Session) StartRun(prompt string) (*Run, error) {
	id := newSessionID()
	root := filepath.Join(s.RunsDir(), id)
	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0755); err != nil {
		return nil, fmt.Errorf("创建 Run 目录失败: %w", err)
	}

	r := &Run{
		ID:        id,
		SessionID: s.ID,
		RootDir:   root,
		Prompt:    prompt,
		StartedAt: time.Now(),
	}
	if err := r.write(); err != nil {
		return nil, err
	}
	return r, nil
}

// Finish marks the run as completed and rewrites run metadata.
func (r *Run) Finish() error {
	now := time.Now()
	r.EndedAt = &now
	return r.write()
}

// MetricsPath returns the run-local metrics path.
func (r *Run) MetricsPath() string {
	return filepath.Join(r.RootDir, "metrics.jsonl")
}

// TracePath returns the run-local trace path.
func (r *Run) TracePath() string {
	return filepath.Join(r.RootDir, "trace.jsonl")
}

// ArtifactsDir returns the run-local artifacts directory.
func (r *Run) ArtifactsDir() string {
	return filepath.Join(r.RootDir, "artifacts")
}

func (r *Run) write() error {
	if err := writeJSON(filepath.Join(r.RootDir, "run.json"), r); err != nil {
		return fmt.Errorf("写入 Run 元数据失败: %w", err)
	}
	return nil
}
