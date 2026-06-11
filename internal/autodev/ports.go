package autodev

import (
	"context"
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
	StagePrompt(command, args string) (string, error)
}

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

// GitRunner executes git with the given arguments in dir and returns the
// combined output. The control plane uses it only for worktree
// infrastructure (worktree add/remove) and read-only verification queries
// (rev-parse, status, ls-remote); it never runs commit/push — the core
// Agent performs all development mutations (REQ-019, REQ-029).
type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) (string, error)
}

// ExecRunner executes an arbitrary program in dir and returns the combined
// output. The control plane uses it for the completion gate (go build/test,
// gofmt) and read-only gh queries (gh ... --json); never for gh mutations.
type ExecRunner interface {
	Run(ctx context.Context, dir string, name string, args ...string) (string, error)
}

// Clock abstracts time for ledger timestamps so tests are deterministic.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock backed by time.Now.
type SystemClock struct{}

// Now returns the current wall-clock time.
func (SystemClock) Now() time.Time { return time.Now() }
