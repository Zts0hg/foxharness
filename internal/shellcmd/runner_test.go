package shellcmd

import (
	"context"
	"strings"
	"testing"
	"time"
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
	if result.Err != context.DeadlineExceeded {
		t.Fatalf("Err = %v, want context deadline exceeded", result.Err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Run returned after %s, want prompt process-tree cancellation", elapsed)
	}
}
