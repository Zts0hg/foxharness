package autodev

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

const (
	maxCommandStreamBytes = 1 << 20
	queryTimeout          = 30 * time.Second
	gateTimeout           = 10 * time.Minute
	stageAttemptTimeout   = 30 * time.Minute
	commandTerminateGrace = 250 * time.Millisecond
	commandReapTimeout    = 5 * time.Second
)

type boundedCommandStream struct {
	mu       sync.Mutex
	data     []byte
	overflow bool
}

func (w *boundedCommandStream) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := maxCommandStreamBytes - len(w.data)
	if remaining > 0 {
		keep := len(p)
		if keep > remaining {
			keep = remaining
		}
		w.data = append(w.data, p[:keep]...)
	}
	if len(p) > remaining {
		w.overflow = true
	}
	return len(p), nil
}

func (w *boundedCommandStream) snapshot() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.data), w.overflow
}

// ExecGitRunner is the production GitRunner backed by a supervised process
// tree. Read-only queries receive a 30-second default; mutating worktree
// operations receive one stage-attempt budget.
type ExecGitRunner struct{}

func NewExecGitRunner() *ExecGitRunner { return &ExecGitRunner{} }

var _ GitRunner = (*ExecGitRunner)(nil)

func (r *ExecGitRunner) Run(ctx context.Context, dir string, args ...string) (CommandResult, error) {
	runCtx, cancel := withDefaultTimeout(ctx, gitCommandTimeout(args))
	defer cancel()
	return runSupervisedCommand(runCtx, dir, "git", args...)
}

// ExecCommandRunner serves quality gates and read-only GitHub queries.
type ExecCommandRunner struct{}

func NewExecCommandRunner() *ExecCommandRunner { return &ExecCommandRunner{} }

var _ ExecRunner = (*ExecCommandRunner)(nil)

func (r *ExecCommandRunner) Run(ctx context.Context, dir string, name string, args ...string) (CommandResult, error) {
	runCtx, cancel := withDefaultTimeout(ctx, execCommandTimeout(name))
	defer cancel()
	return runSupervisedCommand(runCtx, dir, name, args...)
}

func gitCommandTimeout(args []string) time.Duration {
	if len(args) > 1 && args[0] == "worktree" {
		switch args[1] {
		case "add", "remove", "prune":
			return stageAttemptTimeout
		}
	}
	return queryTimeout
}

func execCommandTimeout(name string) time.Duration {
	if name == "gh" {
		return queryTimeout
	}
	return gateTimeout
}

func withDefaultTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok && time.Until(deadline) <= timeout {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func runSupervisedCommand(ctx context.Context, dir, name string, args ...string) (CommandResult, error) {
	if err := ctx.Err(); err != nil {
		return CommandResult{}, err
	}
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	configureCommandProcess(cmd)
	stdout, stderr := &boundedCommandStream{}, &boundedCommandStream{}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Start(); err != nil {
		return commandResult(stdout, stderr, -1), err
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()

	var waitErr error
	select {
	case waitErr = <-wait:
		cleanupErr := signalCommandProcessTree(cmd, true)
		return commandResult(stdout, stderr, exitCode(waitErr)), errors.Join(waitErr, cleanupErr)
	case <-ctx.Done():
		cleanupErr := signalCommandProcessTree(cmd, false)
		timer := time.NewTimer(commandTerminateGrace)
		select {
		case waitErr = <-wait:
			if !timer.Stop() {
				<-timer.C
			}
			cleanupErr = errors.Join(cleanupErr, signalCommandProcessTree(cmd, true))
		case <-timer.C:
			cleanupErr = errors.Join(cleanupErr, signalCommandProcessTree(cmd, true))
			cleanupCtx, cancel := context.WithTimeout(context.Background(), commandReapTimeout)
			defer cancel()
			select {
			case waitErr = <-wait:
			case <-cleanupCtx.Done():
				return commandResult(stdout, stderr, -1), errors.Join(ctx.Err(), cleanupErr,
					fmt.Errorf("command process tree was not reaped within %s", commandReapTimeout))
			}
		}
		return commandResult(stdout, stderr, exitCode(waitErr)), errors.Join(ctx.Err(), cleanupErr)
	}
}

func commandResult(stdout, stderr *boundedCommandStream, code int) CommandResult {
	out, outOverflow := stdout.snapshot()
	errOut, errOverflow := stderr.snapshot()
	return CommandResult{Stdout: out, Stderr: errOut, StdoutOverflow: outOverflow, StderrOverflow: errOverflow, ExitCode: code}
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
