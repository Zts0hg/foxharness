package benchmark

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/session"
)

type HarnessFactory func(ctx context.Context, workDir string, c *Case) (*engine.AgentEngine, *session.Session, error)

type Runner struct {
	factory HarnessFactory
}

func NewRunner(factory HarnessFactory) *Runner {
	return &Runner{factory: factory}
}

type Result struct {
	CaseID      string             `json:"case_id"`
	Success     bool               `json:"success"`
	WorkSpace   string             `json:"workspace"`
	SessionID   string             `json:"session_id"`
	DurationMS  int64              `json:"duration_ms"`
	Error       string             `json:"error,omitempty"`
	Validations []ValidationResult `json:"validations"`
}

func (r *Runner) RunCase(ctx context.Context, c *Case) (*Result, error) {
	workspace, err := os.MkdirTemp("", "foxharness-benchmark-*")
	if err != nil {
		return nil, err
	}

	if err := copyDir(c.Fixture, workspace); err != nil {
		return nil, fmt.Errorf("复制 Fixture 失败: %w", err)
	}

	eng, sess, err := r.factory(ctx, workspace, c)
	if err != nil {
		return nil, fmt.Errorf("创建 Harness 失败: %w", err)
	}

	result := &Result{
		CaseID:    c.ID,
		WorkSpace: workspace,
		SessionID: sess.ID,
	}

	started := time.Now()
	runResult, err := eng.Run(ctx, sess, c.Prompt)
	result.DurationMS = time.Since(started).Milliseconds()

	if runResult != nil {
		result.SessionID = runResult.SessionID
	}

	if err != nil {
		result.Error = err.Error()
	}

	validationResults := ValidateAll(ctx, workspace, c.Validations)
	result.Validations = validationResults
	result.Success = err == nil && allPassed(validationResults)

	return result, nil
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		in, err := os.Open(path)
		if err != nil {
			return err
		}

		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
		if err != nil {
			_ = in.Close()
			return err
		}

		_, copyErr := io.Copy(out, in)
		closeInErr := in.Close()
		closeOutErr := out.Close()

		if copyErr != nil {
			return copyErr
		}

		if closeInErr != nil {
			return closeInErr
		}

		return closeOutErr
	})
}
