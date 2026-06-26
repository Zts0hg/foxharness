package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunBashCommandReturnsOutputWithoutModelToolFormatting(t *testing.T) {
	result := RunBashCommand(context.Background(), t.TempDir(), "printf stdout; printf stderr >&2", time.Second)

	if result.TimedOut {
		t.Fatalf("TimedOut = true, want false")
	}
	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.Output != "stdoutstderr" {
		t.Fatalf("Output = %q, want combined stdout/stderr", result.Output)
	}
}

func TestRunBashCommandPreservesSignificantWhitespace(t *testing.T) {
	result := RunBashCommand(context.Background(), t.TempDir(), "printf '  key: value\\n\\n'", time.Second)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if result.Output != "  key: value\n\n" {
		t.Fatalf("Output = %q, want leading spaces and trailing newlines preserved", result.Output)
	}
}

func TestRunBashCommandBoundsBufferedOutput(t *testing.T) {
	result := RunBashCommand(context.Background(), t.TempDir(), "yes x | head -c 20000", time.Second)

	if result.Err != nil {
		t.Fatalf("Err = %v, want nil", result.Err)
	}
	if !result.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
	if len(result.Output) > MaxBashOutputBytes {
		t.Fatalf("len(Output) = %d, want <= %d", len(result.Output), MaxBashOutputBytes)
	}
}

func TestRunBashCommandReportsNonZeroExitWithOutput(t *testing.T) {
	result := RunBashCommand(context.Background(), t.TempDir(), "printf nope; exit 7", time.Second)

	if result.ExitCode != 7 {
		t.Fatalf("ExitCode = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(result.Output, "nope") {
		t.Fatalf("Output = %q, want command output preserved", result.Output)
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want exit error")
	}
}

func TestRunBashCommandTimesOut(t *testing.T) {
	result := RunBashCommand(context.Background(), t.TempDir(), "sleep 1", 10*time.Millisecond)

	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
	if result.Err == nil {
		t.Fatalf("Err = nil, want timeout error")
	}
}

func TestRunBashCommandTimeoutKillsShellChildProcess(t *testing.T) {
	start := time.Now()
	result := RunBashCommand(context.Background(), t.TempDir(), "sleep 2; printf done", 20*time.Millisecond)
	elapsed := time.Since(start)

	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("RunBashCommand returned after %s, want timeout to kill child process group promptly", elapsed)
	}
	if strings.Contains(result.Output, "done") {
		t.Fatalf("Output = %q, command continued after timeout", result.Output)
	}
}

func TestRunBashCommandContextCancelStopsInfiniteOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan BashCommandResult, 1)

	go func() {
		done <- RunBashCommand(ctx, t.TempDir(), "yes", time.Minute)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case result := <-done:
		if result.Err == nil {
			t.Fatalf("Err = nil, want cancellation error")
		}
		if result.TimedOut {
			t.Fatalf("TimedOut = true, want context cancellation")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunBashCommand did not return promptly after context cancellation")
	}
}
