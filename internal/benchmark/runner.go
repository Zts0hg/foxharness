// Package benchmark provides a framework for running automated benchmark cases
// against the foxharness agent engine. Each case defines a prompt, a fixture
// directory, and a set of validations that determine success. Results are
// collected into structured reports suitable for JSON serialization and
// human-readable summaries.
package benchmark

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/session"
)

// HarnessFactory creates a benchmark harness for a case, allowing callers to
// customize engine configuration per case.
type HarnessFactory func(ctx context.Context, workDir string, c *Case) (*Harness, error)

// Harness contains the engine/session pair used by a benchmark run and the
// runtime fidelity metadata that will be copied into the benchmark result.
type Harness struct {
	Engine          *engine.AgentEngine
	Session         *session.Session
	RuntimeFidelity RuntimeFidelity
}

// Runner executes benchmark cases using a caller-provided HarnessFactory.
type Runner struct {
	factory     HarnessFactory
	caseTimeout func(*Case) time.Duration
}

const defaultCaseTimeout = 10 * time.Minute

// NewRunner creates a Runner that delegates engine creation to the given factory.
func NewRunner(factory HarnessFactory) *Runner {
	return &Runner{factory: factory}
}

// Result captures the outcome of a single benchmark case execution, including
// timing, validation details, and any error that terminated the run.
type Result struct {
	CaseID              string             `json:"case_id"`
	Success             bool               `json:"success"`
	Status              ResultStatus       `json:"status"`
	Workspace           string             `json:"workspace"`
	SessionID           string             `json:"session_id"`
	DurationMS          int64              `json:"duration_ms"`
	Error               string             `json:"error,omitempty"`
	RuntimeError        string             `json:"runtime_error,omitempty"`
	EvaluationError     string             `json:"evaluation_error,omitempty"`
	InfrastructureError string             `json:"infrastructure_error,omitempty"`
	Validations         []ValidationResult `json:"validations"`
	RuntimeFidelity     RuntimeFidelity    `json:"runtime_fidelity"`
}

// ResultStatus identifies the terminal state of one accepted benchmark repeat.
type ResultStatus string

const (
	ResultStatusCompleted            ResultStatus = "completed"
	ResultStatusFailed               ResultStatus = "failed"
	ResultStatusCancelled            ResultStatus = "cancelled"
	ResultStatusTimedOut             ResultStatus = "timed_out"
	ResultStatusInfrastructureFailed ResultStatus = "infrastructure_failed"
)

// RuntimeFidelity records which product runtime invariants a benchmark shares
// and which differences are intentional for benchmark execution.
type RuntimeFidelity struct {
	SharedInvariants       []string `json:"shared_invariants"`
	IntentionalDifferences []string `json:"intentional_differences"`
	Warning                string   `json:"warning,omitempty"`
}

// RunCase copies the case fixture into a temporary workspace, runs the agent
// engine via the configured factory, and validates the results. It returns a
// Result regardless of whether the engine run itself errored; the Success
// field reflects both engine completion and validation outcomes.
func (r *Runner) RunCase(ctx context.Context, c *Case) (*Result, error) {
	caseCtx, cancelCase := context.WithTimeout(ctx, r.timeoutFor(c))
	defer cancelCase()
	result := &Result{CaseID: c.ID}

	workspace, err := os.MkdirTemp("", "foxharness-benchmark-*")
	if err != nil {
		return infrastructureFailure(result, err)
	}
	result.Workspace = workspace

	if err := copyDirContext(caseCtx, c.Fixture, workspace); err != nil {
		if caseCtx.Err() != nil {
			return contextFailure(result, caseCtx.Err()), nil
		}
		return infrastructureFailure(result, fmt.Errorf("复制 Fixture 失败: %w", err))
	}

	harness, err := r.factory(caseCtx, workspace, c)
	if err != nil {
		if caseCtx.Err() != nil {
			return contextFailure(result, caseCtx.Err()), nil
		}
		return infrastructureFailure(result, fmt.Errorf("创建 Harness 失败: %w", err))
	}
	if harness == nil || harness.Engine == nil || harness.Session == nil {
		return infrastructureFailure(result, fmt.Errorf("创建 Harness 失败: harness missing engine or session"))
	}

	result.SessionID = harness.Session.ID
	result.RuntimeFidelity = harness.RuntimeFidelity

	started := time.Now()
	runResult, err := harness.Engine.Run(caseCtx, harness.Session, c.Prompt)
	result.DurationMS = time.Since(started).Milliseconds()

	if runResult != nil {
		result.SessionID = runResult.SessionID
	}

	if err != nil {
		result.Error = err.Error()
		result.RuntimeError = err.Error()
	}

	validationResults := ValidateAll(caseCtx, workspace, c.Validations)
	result.Validations = validationResults
	validationsPassed := allPassed(validationResults)
	if !validationsPassed {
		result.EvaluationError = "one or more validations failed"
	}
	if caseCtx.Err() != nil {
		contextFailure(result, caseCtx.Err())
	} else if err != nil || !validationsPassed {
		result.Status = ResultStatusFailed
	} else {
		result.Status = ResultStatusCompleted
		result.Success = true
	}

	return result, nil
}

func contextFailure(result *Result, err error) *Result {
	result.Success = false
	result.Error = err.Error()
	result.RuntimeError = err.Error()
	result.Status = ResultStatusCancelled
	if errors.Is(err, context.DeadlineExceeded) {
		result.Status = ResultStatusTimedOut
	}
	return result
}

func infrastructureFailure(result *Result, err error) (*Result, error) {
	result.Success = false
	result.Status = ResultStatusInfrastructureFailed
	result.Error = err.Error()
	result.InfrastructureError = err.Error()
	return result, err
}

func (r *Runner) timeoutFor(c *Case) time.Duration {
	if r.caseTimeout != nil {
		return r.caseTimeout(c)
	}
	if c != nil && c.TimeoutSeconds > 0 {
		return time.Duration(c.TimeoutSeconds) * time.Second
	}
	return defaultCaseTimeout
}

func copyDir(src, dst string) error {
	return copyDirContext(context.Background(), src, dst)
}

func copyDirContext(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
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
