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

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

/* HarnessFactory creates a benchmark runtime harness for one immutable case. */
type HarnessFactory func(ctx context.Context, workDir string, c *Case) (*Harness, error)

/* Harness contains one runtime-owned session and its immutable benchmark run input. */
type Harness struct {
	Session         *foxruntime.AgentSession
	RunSpec         foxruntime.RunSpec
	RuntimeFidelity RuntimeFidelity
}

// Runner executes benchmark cases using a caller-provided HarnessFactory.
type Runner struct {
	factory         HarnessFactory
	caseTimeout     func(*Case) time.Duration
	cleanupTimeout  func() time.Duration
	createWorkspace func() (string, error)
	removeWorkspace func(context.Context, string) error
	closeSession    func(*foxruntime.AgentSession, time.Duration) error
}

const (
	defaultCaseTimeout    = 10 * time.Minute
	defaultCleanupTimeout = 30 * time.Second
)

/* NewRunner creates a Runner that delegates runtime composition to the given factory. */
func NewRunner(factory HarnessFactory) *Runner {
	return &Runner{factory: factory}
}

// Result captures the outcome of a single benchmark case execution, including
// timing, validation details, and any error that terminated the run.
type Result struct {
	SchemaVersion       int                `json:"schema_version"`
	CaseID              string             `json:"case_id"`
	RepeatIndex         int                `json:"repeat_index"`
	RunID               string             `json:"run_id"`
	CaseDefinitionID    string             `json:"case_definition_id"`
	FixtureID           string             `json:"fixture_id"`
	Success             bool               `json:"success"`
	Status              ResultStatus       `json:"status"`
	TerminalCause       string             `json:"terminal_cause,omitempty"`
	RuntimeStatus       RuntimeStatus      `json:"runtime_status"`
	RuntimeCause        string             `json:"runtime_cause,omitempty"`
	ProviderProtocol    string             `json:"provider_protocol"`
	Model               string             `json:"model"`
	CaseDeadline        time.Time          `json:"case_deadline"`
	Workspace           string             `json:"workspace"`
	SessionID           string             `json:"session_id"`
	DurationMS          int64              `json:"duration_ms"`
	Error               string             `json:"error,omitempty"`
	RuntimeError        string             `json:"runtime_error,omitempty"`
	EvaluationError     string             `json:"evaluation_error,omitempty"`
	InfrastructureError string             `json:"infrastructure_error,omitempty"`
	CleanupError        string             `json:"cleanup_error,omitempty"`
	Validations         []ValidationResult `json:"validations"`
	RuntimeFidelity     RuntimeFidelity    `json:"runtime_fidelity"`
}

// ResultSchemaVersion identifies the corrected benchmark result schema.
const ResultSchemaVersion = 1

// ResultStatus identifies the terminal state of one accepted benchmark repeat.
type ResultStatus string

const (
	ResultStatusCompleted            ResultStatus = "completed"
	ResultStatusFailed               ResultStatus = "failed"
	ResultStatusCancelled            ResultStatus = "cancelled"
	ResultStatusTimedOut             ResultStatus = "timed_out"
	ResultStatusInfrastructureFailed ResultStatus = "infrastructure_failed"
)

// RuntimeStatus identifies the terminal state of the Agent runtime independently of evaluation.
type RuntimeStatus string

const (
	RuntimeStatusNotStarted RuntimeStatus = "not_started"
	RuntimeStatusCompleted  RuntimeStatus = "completed"
	RuntimeStatusFailed     RuntimeStatus = "failed"
	RuntimeStatusCancelled  RuntimeStatus = "cancelled"
	RuntimeStatusTimedOut   RuntimeStatus = "timed_out"
)

// RuntimeFidelity records which product runtime invariants a benchmark shares
// and which differences are intentional for benchmark execution.
type RuntimeFidelity struct {
	Spec                   BenchmarkRuntimeSpec `json:"spec"`
	SharedInvariants       []string             `json:"shared_invariants"`
	IntentionalDifferences []string             `json:"intentional_differences"`
	Warning                string               `json:"warning,omitempty"`
}

// RunCase copies the case fixture into a temporary workspace, runs the agent
// runtime via the configured factory, and validates the results. It returns a
// Result regardless of whether the engine run itself errored; the Success
// field reflects both engine completion and validation outcomes.
func (r *Runner) RunCase(ctx context.Context, c *Case) (*Result, error) {
	return r.RunRepeat(ctx, c, 1)
}

// RunRepeat executes one case with its one-based repeat identity.
func (r *Runner) RunRepeat(ctx context.Context, c *Case, repeatIndex int) (returnedResult *Result, returnedErr error) {
	result := &Result{
		SchemaVersion: ResultSchemaVersion,
		RepeatIndex:   repeatIndex,
		RuntimeStatus: RuntimeStatusNotStarted,
	}
	if c == nil {
		return infrastructureFailure(result, fmt.Errorf("benchmark case 不能为 nil"))
	}
	result.CaseID = c.ID
	if repeatIndex <= 0 {
		return infrastructureFailure(result, fmt.Errorf("benchmark repeat index 必须从 1 开始"))
	}
	caseSnapshot := cloneCase(c)
	caseCtx, cancelCase := context.WithTimeout(ctx, r.timeoutFor(caseSnapshot))
	defer cancelCase()
	if deadline, ok := caseCtx.Deadline(); ok {
		result.CaseDeadline = deadline
	}
	workspace, err := r.createWorkspaceOrDefault()
	if err != nil {
		return infrastructureFailure(result, err)
	}
	result.Workspace = workspace
	defer func() {
		if result.Success {
			return
		}
		cleanupCtx, cancelCleanup := context.WithTimeout(context.Background(), r.cleanupTimeoutOrDefault())
		defer cancelCleanup()
		if err := r.removeWorkspaceWithContext(cleanupCtx, workspace); err != nil {
			cleanupErr := fmt.Errorf("清理 benchmark workspace 失败: %w", err)
			result.Success = false
			result.Status = ResultStatusInfrastructureFailed
			result.CleanupError = cleanupErr.Error()
			if result.InfrastructureError == "" {
				result.InfrastructureError = cleanupErr.Error()
			} else {
				result.InfrastructureError = errors.Join(errors.New(result.InfrastructureError), cleanupErr).Error()
			}
			result.Error = result.InfrastructureError
			result.TerminalCause = result.InfrastructureError
			returnedResult = result
			returnedErr = errors.Join(returnedErr, cleanupErr)
		}
	}()

	if err := copyDirContext(caseCtx, caseSnapshot.Fixture, workspace); err != nil {
		if caseCtx.Err() != nil {
			return contextFailure(result, caseCtx.Err()), nil
		}
		return infrastructureFailure(result, fmt.Errorf("复制 Fixture 失败: %w", err))
	}
	fixtureID, err := fixtureTreeID(caseCtx, workspace)
	if err != nil {
		if caseCtx.Err() != nil {
			return contextFailure(result, caseCtx.Err()), nil
		}
		return infrastructureFailure(result, fmt.Errorf("计算 fixture identity 失败: %w", err))
	}
	result.FixtureID = fixtureID
	caseID, err := caseDefinitionID(caseSnapshot, fixtureID)
	if err != nil {
		return infrastructureFailure(result, fmt.Errorf("计算 case definition identity 失败: %w", err))
	}
	result.CaseDefinitionID = caseID

	harness, err := r.factory(caseCtx, workspace, caseSnapshot)
	if err != nil {
		if caseCtx.Err() != nil {
			return contextFailure(result, caseCtx.Err()), nil
		}
		return infrastructureFailure(result, fmt.Errorf("创建 Harness 失败: %w", err))
	}
	if harness == nil || harness.Session == nil {
		return infrastructureFailure(result, fmt.Errorf("创建 Harness 失败: harness missing runtime session"))
	}

	result.SessionID = string(harness.Session.Snapshot().ID)
	result.RuntimeFidelity = harness.RuntimeFidelity
	result.ProviderProtocol = harness.RuntimeFidelity.Spec.ProviderProtocol
	result.Model = harness.RuntimeFidelity.Spec.Model

	started := time.Now()
	runSpec := harness.RunSpec
	runSpec.Prompt = caseSnapshot.Prompt
	runResult, err := harness.Session.Run(caseCtx, runSpec)
	result.DurationMS = time.Since(started).Milliseconds()
	if runResult.SessionID != "" {
		result.SessionID = string(runResult.SessionID)
	}
	if runResult.RunID != "" {
		result.RunID = string(runResult.RunID)
	}
	if err != nil {
		result.Error = err.Error()
		result.RuntimeError = err.Error()
		result.RuntimeCause = err.Error()
		result.RuntimeStatus = RuntimeStatusFailed
		if errors.Is(err, context.Canceled) || errors.Is(caseCtx.Err(), context.Canceled) {
			result.RuntimeStatus = RuntimeStatusCancelled
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(caseCtx.Err(), context.DeadlineExceeded) {
			result.RuntimeStatus = RuntimeStatusTimedOut
		}
	} else {
		result.RuntimeStatus = RuntimeStatusCompleted
	}
	if cleanupErr := r.closeRuntimeSession(harness.Session); cleanupErr != nil {
		/* A close or recovery failure is evidence, not a verdict: the run
		 * already finished, so the repeat keeps its outcome and the remaining
		 * repeats still execute. */
		result.CleanupError = cleanupErr.Error()
		if result.InfrastructureError == "" {
			result.InfrastructureError = cleanupErr.Error()
		} else {
			result.InfrastructureError = errors.Join(errors.New(result.InfrastructureError), cleanupErr).Error()
		}
	}

	/* The close window above can outlive the case deadline; a run that
	 * already completed still gets its evaluation instead of failing every
	 * validation against a consumed deadline. A cancelled parent cancels the
	 * case, and no new validation work starts for it. */
	validateCtx := caseCtx
	if errors.Is(caseCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil && err == nil {
		validateCtx = context.WithoutCancel(caseCtx)
	}
	validationResults := ValidateAll(validateCtx, workspace, caseSnapshot.Validations)
	result.Validations = validationResults
	validationsPassed := allPassed(validationResults)
	if !validationsPassed {
		result.EvaluationError = "one or more validations failed"
	}
	/* An expired case deadline must not consume a run that already completed:
	 * the close window above runs after the verdict evidence exists. */
	if caseCtx.Err() != nil && err != nil {
		contextFailure(result, caseCtx.Err())
	} else if err != nil {
		result.Status = ResultStatusFailed
		if result.RuntimeStatus == RuntimeStatusCancelled {
			result.Status = ResultStatusCancelled
		}
		if result.RuntimeStatus == RuntimeStatusTimedOut {
			result.Status = ResultStatusTimedOut
		}
		result.TerminalCause = result.RuntimeCause
	} else if !validationsPassed {
		result.Status = ResultStatusFailed
		result.TerminalCause = result.EvaluationError
	} else {
		result.Status = ResultStatusCompleted
		result.Success = true
	}

	return result, nil
}

func (r *Runner) closeRuntimeSession(agentSession *foxruntime.AgentSession) error {
	if r.closeSession != nil {
		return r.closeSession(agentSession, r.cleanupTimeoutOrDefault())
	}
	return closeRuntimeSession(agentSession, r.cleanupTimeoutOrDefault())
}

func closeRuntimeSession(agentSession *foxruntime.AgentSession, timeout time.Duration) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	recoverErr := agentSession.RecoverRunFinish(cleanupCtx)
	closeErr := agentSession.Close(cleanupCtx)
	if recoverErr == nil && closeErr == nil {
		return nil
	}
	return fmt.Errorf("清理 benchmark runtime session 失败: %w", errors.Join(recoverErr, closeErr))
}

func cloneCase(c *Case) *Case {
	copy := *c
	copy.Validations = append([]Validation(nil), c.Validations...)
	return &copy
}

func (r *Runner) cleanupTimeoutOrDefault() time.Duration {
	if r.cleanupTimeout != nil {
		return r.cleanupTimeout()
	}
	return defaultCleanupTimeout
}

func (r *Runner) createWorkspaceOrDefault() (string, error) {
	if r.createWorkspace != nil {
		return r.createWorkspace()
	}
	return os.MkdirTemp("", "foxharness-benchmark-*")
}

func (r *Runner) removeWorkspaceWithContext(ctx context.Context, workspace string) error {
	if r.removeWorkspace != nil {
		return r.removeWorkspace(ctx, workspace)
	}
	return removeAllContext(ctx, workspace)
}

func contextFailure(result *Result, err error) *Result {
	result.Success = false
	result.Error = err.Error()
	result.Status = ResultStatusCancelled
	result.TerminalCause = err.Error()
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
	result.TerminalCause = err.Error()
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
	sourceInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		return fmt.Errorf("fixture root must be a directory, not a symlink or other file type")
	}
	sourceRoot, err := os.OpenRoot(src)
	if err != nil {
		return err
	}
	defer sourceRoot.Close()
	destinationRoot, err := os.OpenRoot(dst)
	if err != nil {
		return err
	}
	defer destinationRoot.Close()

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

		if d.IsDir() {
			return destinationRoot.MkdirAll(rel, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("fixture symlink is not allowed: %s", rel)
		}
		info, err := sourceRoot.Lstat(rel)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("fixture entry is not a regular file: %s", rel)
		}

		in, err := sourceRoot.Open(rel)
		if err != nil {
			return err
		}
		openedInfo, err := in.Stat()
		if err != nil || !openedInfo.Mode().IsRegular() {
			_ = in.Close()
			if err != nil {
				return err
			}
			return fmt.Errorf("fixture target is not a regular file: %s", rel)
		}
		finalInfo, err := sourceRoot.Lstat(rel)
		if err != nil || finalInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openedInfo, finalInfo) {
			_ = in.Close()
			if err != nil {
				return err
			}
			return fmt.Errorf("fixture entry changed while being opened: %s", rel)
		}

		out, err := destinationRoot.OpenFile(rel, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
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

func removeAllContext(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		child := filepath.Join(path, entry.Name())
		if entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			if err := removeAllContext(ctx, child); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(child); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Remove(path)
}
