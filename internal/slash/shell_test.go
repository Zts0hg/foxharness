package slash

import (
	"strings"
	"testing"
	"time"
)

func TestExecuteEmbeddedShell_Success(t *testing.T) {
	got, err := ExecuteEmbeddedShell("hello "+"!`echo world`", "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteEmbeddedShell_Failure(t *testing.T) {
	got, err := ExecuteEmbeddedShell("x = "+"!`false`", "", 5*time.Second)
	if err != nil {
		t.Fatalf("ExecuteEmbeddedShell returned err for non-fatal failure: %v", err)
	}
	if !strings.Contains(got, "[ERROR:") {
		t.Errorf("expected ERROR marker, got %q", got)
	}
}

func TestExecuteEmbeddedShell_NonexistentCommand(t *testing.T) {
	got, err := ExecuteEmbeddedShell("a "+"!`nonexistent-command-foo`", "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "[ERROR:") {
		t.Errorf("expected ERROR marker, got %q", got)
	}
}

func TestExecuteEmbeddedShell_MultipleEmbeddings(t *testing.T) {
	in := "A " + "!`echo one`" + " B " + "!`echo two`" + " C"
	got, err := ExecuteEmbeddedShell(in, "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "A one B two C" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteEmbeddedShell_NoEmbeddings(t *testing.T) {
	in := "plain content, no shell"
	got, err := ExecuteEmbeddedShell(in, "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != in {
		t.Errorf("got %q want unchanged", got)
	}
}

func TestExecuteEmbeddedShell_EmptyOutput(t *testing.T) {
	got, err := ExecuteEmbeddedShell("pre"+"!`true`"+"post", "", 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "prepost" {
		t.Errorf("got %q", got)
	}
}

func TestExecuteEmbeddedShell_Timeout(t *testing.T) {
	got, err := ExecuteEmbeddedShell("x="+"!`sleep 5`", "", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, "[ERROR:") {
		t.Errorf("expected ERROR marker for timeout, got %q", got)
	}
}

func TestExecuteEmbeddedShell_WorkDir(t *testing.T) {
	wd := t.TempDir()
	got, err := ExecuteEmbeddedShell("dir="+"!`pwd`", wd, 5*time.Second)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(got, wd) && !strings.HasPrefix(got, "dir=/") {
		t.Errorf("expected workDir in output, got %q", got)
	}
}
