package shellcmd

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/processtree"
	"github.com/Zts0hg/foxharness/internal/toolresult"
)

const (
	defaultTimeout     = 30 * time.Second
	processReapTimeout = 5 * time.Second
	MaxOutputBytes     = toolresult.MaxToolResultBytes
)

/* Result captures a local shell process result before caller-specific formatting. */
type Result struct {
	Output    string
	ExitCode  int
	TimedOut  bool
	Truncated bool
	Err       error
}

/* Run executes command with bash in workDir and returns combined stdout/stderr. */
func Run(ctx context.Context, workDir, command string, timeout time.Duration) Result {
	return run(ctx, workDir, command, timeout, startProcessTree)
}

func startProcessTree(cmd *exec.Cmd) (processtree.Tree, error) { return processtree.Start(cmd) }

func run(ctx context.Context, workDir, command string, timeout time.Duration, start func(*exec.Cmd) (processtree.Tree, error)) Result {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = workDir

	output := newBoundedOutput(MaxOutputBytes)
	cmd.Stdout = output
	cmd.Stderr = output

	tree, err := start(cmd)
	if err != nil {
		return Result{Output: output.String(), Truncated: output.Truncated(), Err: err}
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var cleanupErr error
	select {
	case err = <-wait:
		/* The command completed on its own: reap the direct child and let
		 * detached survivors live instead of killing the whole group. */
		cleanupErr = tree.Release(processReapTimeout)
	case <-timeoutCtx.Done():
		cleanupErr = tree.Signal(true)
		select {
		case waitErr := <-wait:
			/* The killed process's own exit error is the result the baseline
			 * surfaced for a killed command; the cancellation context only
			 * takes over when no exit could be observed. */
			var exitErr *exec.ExitError
			if errors.As(waitErr, &exitErr) {
				err = waitErr
			} else {
				err = timeoutCtx.Err()
			}
		case <-time.After(processReapTimeout):
			err = timeoutCtx.Err()
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("shell process tree was not reaped within %s", processReapTimeout))
		}
		cleanupErr = errors.Join(cleanupErr, tree.Close(processReapTimeout))
	}
	if cleanupErr != nil {
		err = errors.Join(err, cleanupErr)
	}
	result := Result{
		Output:    output.String(),
		Truncated: output.Truncated(),
		Err:       err,
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	}
	if timeoutCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Err = errors.Join(result.Err, timeoutCtx.Err())
	}
	return result
}

type boundedOutput struct {
	mu        sync.Mutex
	limit     int
	buf       []byte
	truncated bool
}

func newBoundedOutput(limit int) *boundedOutput {
	return &boundedOutput{limit: limit}
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.limit <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(b.buf) < b.limit {
		remaining := b.limit - len(b.buf)
		if len(p) <= remaining {
			b.buf = append(b.buf, p...)
		} else {
			b.buf = append(b.buf, p[:remaining]...)
			b.truncated = true
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *boundedOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *boundedOutput) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
