package shellcmd

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/toolresult"
)

const (
	defaultTimeout = 30 * time.Second
	MaxOutputBytes = toolresult.MaxToolResultBytes
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
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", command)
	cmd.Dir = workDir
	ConfigureCommand(cmd)

	output := newBoundedOutput(MaxOutputBytes)
	cmd.Stdout = output
	cmd.Stderr = output

	err := cmd.Run()
	result := Result{
		Output:    output.String(),
		Truncated: output.Truncated(),
		Err:       err,
	}
	if timeoutCtx.Err() == context.DeadlineExceeded {
		result.TimedOut = true
		result.Err = timeoutCtx.Err()
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
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
