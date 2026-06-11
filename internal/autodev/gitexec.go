package autodev

import (
	"context"
	"os/exec"
)

// ExecGitRunner is the production GitRunner backed by os/exec. The control
// plane invokes it only for worktree infrastructure and read-only
// verification queries (rev-parse, status, ls-remote); every development
// mutation — commit, push, issue, PR — is performed by the core Agent
// through its own tools, never here (REQ-019, Decision 4).
type ExecGitRunner struct{}

// NewExecGitRunner returns the os/exec-backed GitRunner.
func NewExecGitRunner() *ExecGitRunner { return &ExecGitRunner{} }

var _ GitRunner = (*ExecGitRunner)(nil)

// Run executes git with args in dir, returning the combined output raw so
// callers parse it themselves.
func (r *ExecGitRunner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ExecCommandRunner is the production ExecRunner backed by os/exec. It
// serves the completion gate (go build/test, gofmt) and read-only gh
// queries (gh ... --json); it runs no gh mutations.
type ExecCommandRunner struct{}

// NewExecCommandRunner returns the os/exec-backed ExecRunner.
func NewExecCommandRunner() *ExecCommandRunner { return &ExecCommandRunner{} }

var _ ExecRunner = (*ExecCommandRunner)(nil)

// Run executes name with args in dir, returning the combined output raw.
func (r *ExecCommandRunner) Run(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
