package autodev

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// CoreRunner is the execution-plane seam over the core Agent. internal/app
// adapts *app.AgentRunner to this interface; tests inject deterministic
// fakes. One CoreRunner is created per item, scoped to its worktree.
type CoreRunner interface {
	// Run executes one prompt to completion and returns the run result.
	Run(ctx context.Context, prompt string, r engine.Reporter) (*engine.RunResult, error)
	// Drain waits for post-run work from every completed Run. Callers invoke
	// it before another Run so memory visibility is scheduler-independent.
	Drain(ctx context.Context) error
	// Close cancels outstanding post-run work and joins the item-scoped
	// runtime before its worktree can be removed.
	Close(ctx context.Context) error
	// SetUserAsker installs the asker answering ask_user_question calls;
	// autodev installs an EngineerAsker so no human is required (REQ-013).
	SetUserAsker(a tools.UserAsker)
	// SetModel switches the model used by future runs (REQ-016).
	SetModel(model string) error
	// WorkDir reports the runner's working directory (the item worktree).
	WorkDir() string
	// StagePrompt materializes a codexspec command body (e.g.
	// "codexspec:generate-spec") with args via the runner's slash
	// registry and executor, returning the processed prompt (REQ-009).
	// ctx bounds any embedded-shell processing in the command body.
	StagePrompt(ctx context.Context, command, args string) (string, error)
}

// CoreLifecycleError reports post-run work that could not be joined. The
// owning item must retain its worktree and stop dependent transitions.
type CoreLifecycleError struct {
	Operation string
	Err       error
}

func (e *CoreLifecycleError) Error() string {
	return fmt.Sprintf("core lifecycle %s: %v", e.Operation, e.Err)
}

func (e *CoreLifecycleError) Unwrap() error { return e.Err }

// CoreRunnerFactory creates a CoreRunner bound to a work directory. The
// orchestrator calls it once per item so every item gets a fresh engine
// session isolated in its own worktree (NFR-003).
type CoreRunnerFactory interface {
	New(ctx context.Context, workDir, model string) (CoreRunner, error)
}

// EngineerAgent is the simulated senior engineer supervising the core Agent.
// It shares the core Agent's model and is read-only with respect to the
// workspace (REQ-016).
type EngineerAgent interface {
	// Decide answers an ask_user_question call: it selects an option label
	// (or labels for multi-select) per question, or supplies "Other" free
	// text when no offered option fits. It never cancels (REQ-013).
	Decide(ctx context.Context, qs []tools.Question, c StageContext) ([]tools.Answer, error)
	// Reply answers a free-form prose question the core Agent ended a turn
	// with; the reply becomes the next user message (REQ-014).
	Reply(ctx context.Context, prompt string, c StageContext) (string, error)
	// Review supervises a finished core run the way a human user would:
	// given the run result and the Go-computed verification gap, it returns
	// "" to approve or a corrective instruction to feed back to the core
	// Agent as the next user message (REQ-014).
	Review(ctx context.Context, res *engine.RunResult, gap string, c StageContext) (string, error)
}

// GitRunner executes git with independently bounded stdout and stderr. The
// control plane uses it only for worktree
// infrastructure (worktree add/remove) and read-only verification queries
// (rev-parse, status, ls-remote); it never runs commit/push — the core
// Agent performs all development mutations (REQ-019, REQ-029).
type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) (CommandResult, error)
}

// ExecRunner executes an arbitrary program in dir with independently bounded
// output streams. The control plane uses it for the completion gate and
// read-only gh queries; never for gh mutations.
type ExecRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error)
}

// CommandResult retains bounded stdout and stderr plus process status.
type CommandResult struct {
	Stdout         string
	Stderr         string
	StdoutOverflow bool
	StderrOverflow bool
	ExitCode       int
}

// Output returns both diagnostic streams without discarding either one.
func (r CommandResult) Output() string {
	if r.Stdout == "" {
		return r.Stderr
	}
	if r.Stderr == "" {
		return r.Stdout
	}
	return r.Stdout + "\n" + r.Stderr
}

// OverflowError returns typed truncation evidence, or nil when both streams
// fit their independent limits.
func (r CommandResult) OverflowError() error {
	if !r.StdoutOverflow && !r.StderrOverflow {
		return nil
	}
	return &CommandOverflowError{Stdout: r.StdoutOverflow, Stderr: r.StderrOverflow}
}

// CommandOverflowError identifies exactly which command streams exceeded
// their capture limits.
type CommandOverflowError struct {
	Stdout bool
	Stderr bool
}

func (e *CommandOverflowError) Error() string {
	var streams []string
	if e.Stdout {
		streams = append(streams, "stdout")
	}
	if e.Stderr {
		streams = append(streams, "stderr")
	}
	return fmt.Sprintf("command %s exceeded the capture limit", strings.Join(streams, " and "))
}

func strictCommandStdout(result CommandResult, runErr error) (string, error) {
	return result.Stdout, errors.Join(runErr, result.OverflowError())
}

// Clock abstracts time for ledger timestamps so tests are deterministic.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock backed by time.Now.
type SystemClock struct{}

// Now returns the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }
