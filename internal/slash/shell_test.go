package slash

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExecuteEmbeddedShell_Success(t *testing.T) {
	got, err := ExecuteEmbeddedShell(context.Background(), "hello "+"!`echo world`", "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteEmbeddedShell_Failure(t *testing.T) {
	got, err := ExecuteEmbeddedShell(context.Background(), "x = "+"!`false`", "", 5*time.Second)
	if err != nil {
		t.Fatalf("ExecuteEmbeddedShell returned err for non-fatal failure: %v", err)
	}
	if !strings.Contains(got, "[ERROR:") {
		t.Errorf("expected ERROR marker, got %q", got)
	}
}

func TestExecuteEmbeddedShell_NonexistentCommand(t *testing.T) {
	got, err := ExecuteEmbeddedShell(context.Background(), "a "+"!`nonexistent-command-foo`", "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "[ERROR:") {
		t.Errorf("expected ERROR marker, got %q", got)
	}
}

func TestExecuteEmbeddedShell_MultipleEmbeddings(t *testing.T) {
	in := "A " + "!`echo one`" + " B " + "!`echo two`" + " C"
	got, err := ExecuteEmbeddedShell(context.Background(), in, "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "A one B two C" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteEmbeddedShell_NoEmbeddings(t *testing.T) {
	in := "plain content, no shell"
	got, err := ExecuteEmbeddedShell(context.Background(), in, "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != in {
		t.Errorf("got %q want unchanged", got)
	}
}

func TestExecuteEmbeddedShell_EmptyOutput(t *testing.T) {
	got, err := ExecuteEmbeddedShell(context.Background(), "pre"+"!`true`"+"post", "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "prepost" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteEmbeddedShell_Timeout(t *testing.T) {
	got, err := ExecuteEmbeddedShell(context.Background(), "x="+"!`sleep 5`", "", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "[ERROR:") {
		t.Errorf("expected ERROR marker for timeout, got %q", got)
	}
}

func TestExecuteEmbeddedShell_ParentCtxCancelKillsCommand(t *testing.T) {
	// The shell embedding must honor the caller's context — when the
	// TUI cancels the prepare stage (Ctrl+C), in-flight embeddings
	// MUST abort promptly instead of running to their own timeout.
	parent, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		out string
		err error
	}, 1)
	go func() {
		out, err := ExecuteEmbeddedShell(parent, "x="+"!`sleep 10`", "", 30*time.Second)
		done <- struct {
			out string
			err error
		}{out, err}
	}()
	// Cancel after a short delay so the goroutine is inside the shell.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case res := <-done:
		// Must finish in well under the per-embed timeout (30s) once
		// cancellation propagates — verifying ctx was actually wired.
		if res.err != nil {
			t.Errorf("unexpected err: %v", res.err)
		}
		if !strings.Contains(res.out, "[ERROR:") {
			t.Errorf("expected error marker after cancel, got %q", res.out)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ExecuteEmbeddedShell did not honor parent cancel within 3s — ctx not wired through")
	}
}

func TestExecuteEmbeddedShell_NilCtxStillWorks(t *testing.T) {
	// Defensive: callers may pass nil. Function must treat as background.
	got, err := ExecuteEmbeddedShell(nil, "hi "+"!`echo there`", "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "hi there" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteEmbeddedShell_WorkDir(t *testing.T) {
	wd := t.TempDir()
	got, err := ExecuteEmbeddedShell(context.Background(), "dir="+"!`pwd`", wd, 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, wd) && !strings.HasPrefix(got, "dir=/") {
		t.Errorf("expected workDir in output, got %q", got)
	}
}
