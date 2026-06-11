package autodev

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func requireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s not available in PATH", name)
	}
}

func TestExecGitRunnerRunsGitInDir(t *testing.T) {
	requireBinary(t, "git")
	dir := t.TempDir()
	git := NewExecGitRunner()
	ctx := context.Background()

	if _, err := git.Run(ctx, dir, "init", "--initial-branch=main"); err != nil {
		// Older gits lack --initial-branch; plain init is fine for the test.
		if _, err := git.Run(ctx, dir, "init"); err != nil {
			t.Fatalf("git init failed: %v", err)
		}
	}

	out, err := git.Run(ctx, dir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		t.Fatalf("git rev-parse returned error: %v", err)
	}
	if strings.TrimSpace(out) != "true" {
		t.Errorf("rev-parse output = %q, want raw output %q for callers to parse", out, "true")
	}
}

func TestExecGitRunnerReturnsErrorOutsideRepo(t *testing.T) {
	requireBinary(t, "git")
	git := NewExecGitRunner()

	out, err := git.Run(context.Background(), t.TempDir(), "rev-parse", "--is-inside-work-tree")
	if err == nil {
		t.Fatalf("rev-parse outside a repo returned nil error (out=%q), want failure", out)
	}
}

func TestExecCommandRunnerReturnsRawOutput(t *testing.T) {
	runner := NewExecCommandRunner()

	out, err := runner.Run(context.Background(), t.TempDir(), "echo", "hello", "world")
	if err != nil {
		t.Fatalf("echo returned error: %v", err)
	}
	if strings.TrimSpace(out) != "hello world" {
		t.Errorf("output = %q, want %q", out, "hello world")
	}
}

func TestExecCommandRunnerPropagatesFailure(t *testing.T) {
	requireBinary(t, "false")
	runner := NewExecCommandRunner()

	if _, err := runner.Run(context.Background(), t.TempDir(), "false"); err == nil {
		t.Fatal("false returned nil error, want exit failure")
	}
}

func TestExecRunnersImplementPorts(t *testing.T) {
	var _ GitRunner = NewExecGitRunner()
	var _ ExecRunner = NewExecCommandRunner()
}
