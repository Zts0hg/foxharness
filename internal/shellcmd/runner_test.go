package shellcmd

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/processtree"
)

func TestRunCapturesCombinedOutputAndExitStatus(t *testing.T) {
	result := Run(context.Background(), t.TempDir(), "printf stdout; printf stderr >&2; exit 7", time.Second)

	if result.Output != "stdoutstderr" {
		t.Fatalf("Output = %q, want %q", result.Output, "stdoutstderr")
	}
	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if result.Err == nil {
		t.Fatal("Err = nil, want non-nil exit error")
	}
}

func TestRunBoundsBufferedOutput(t *testing.T) {
	result := Run(context.Background(), t.TempDir(), "yes x | head -c 405000", time.Second)

	if !result.Truncated {
		t.Fatal("Truncated = false, want true")
	}
	if len(result.Output) > MaxOutputBytes {
		t.Fatalf("len(Output) = %d, want <= %d", len(result.Output), MaxOutputBytes)
	}
	if !strings.HasPrefix(result.Output, "x\n") {
		t.Fatalf("Output prefix = %q, want shell output", result.Output[:min(len(result.Output), 16)])
	}
}

func TestRunTimesOutAndKillsProcessTree(t *testing.T) {
	started := time.Now()
	result := Run(context.Background(), t.TempDir(), "sleep 5; printf done", 20*time.Millisecond)

	if !result.TimedOut {
		t.Fatal("TimedOut = false, want true")
	}
	if !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("Err = %v, want context deadline exceeded", result.Err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Run returned after %s, want prompt process-tree cancellation", elapsed)
	}
}

func TestRunPreservesProcessTreeCleanupFailureAfterTimeout(t *testing.T) {
	cleanupErr := errors.New("process-tree cleanup failed")
	result := run(context.Background(), t.TempDir(), "sleep 5", 20*time.Millisecond, func(cmd *exec.Cmd) (processtree.Tree, error) {
		if err := cmd.Start(); err != nil {
			return nil, err
		}
		return cleanupFailureTree{cmd: cmd, err: cleanupErr}, nil
	})

	if !result.TimedOut || !errors.Is(result.Err, context.DeadlineExceeded) {
		t.Fatalf("timeout result = %#v, want deadline exceeded", result)
	}
	if !errors.Is(result.Err, cleanupErr) {
		t.Fatalf("Err = %v, want cleanup failure %v", result.Err, cleanupErr)
	}
}

type cleanupFailureTree struct {
	cmd *exec.Cmd
	err error
}

func (tree cleanupFailureTree) Signal(bool) error {
	_ = tree.cmd.Process.Kill()
	return tree.err
}

func (cleanupFailureTree) Close(time.Duration) error { return nil }
