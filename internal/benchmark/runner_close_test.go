package benchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

func newCloseTestRunner(t *testing.T, closeSession func(*foxruntime.AgentSession, time.Duration) error) (*Runner, *Case) {
	t.Helper()
	fixture := t.TempDir()
	if err := os.WriteFile(filepath.Join(fixture, "input.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := &Case{
		ID: "close-case", Fixture: fixture, Prompt: "run", MaxTurns: 1, TimeoutSeconds: 60,
		Validations: []Validation{{Type: "file_contains", Path: "input.txt", Contains: "source"}},
	}
	model := &benchmarkProfileProvider{final: "done"}
	runner := NewRunner(func(_ context.Context, workDir string, c *Case) (*Harness, error) {
		return benchmarkProfileHarness(t, workDir, c, model)
	})
	runner.closeSession = closeSession
	return runner, c
}

/* TestBenchmarkCloseFailureKeepsCompletedVerdict pins the baseline verdict
 * policy: a session-close or recovery failure is recorded as evidence but
 * never flips a completed repeat or aborts the remaining repeats. */
func TestBenchmarkCloseFailureKeepsCompletedVerdict(t *testing.T) {
	runner, c := newCloseTestRunner(t, func(*foxruntime.AgentSession, time.Duration) error {
		return errors.New("close unavailable")
	})
	result, err := runner.RunRepeat(context.Background(), c, 1)
	if err != nil {
		t.Fatalf("RunRepeat() error = %v, want the repeats to continue", err)
	}
	defer os.RemoveAll(result.Workspace)
	if !result.Success || result.Status != ResultStatusCompleted {
		t.Fatalf("result = %#v, want the completed verdict", result)
	}
	if result.CleanupError == "" || result.InfrastructureError == "" {
		t.Fatalf("close failure evidence = cleanup:%q infrastructure:%q, want both recorded", result.CleanupError, result.InfrastructureError)
	}
	if len(result.Validations) != 1 || !result.Validations[0].Passed {
		t.Fatalf("validations = %#v, want the evaluation to still run", result.Validations)
	}
}

/* TestBenchmarkCloseWindowDoesNotConsumeCompletedRun pins the deadline
 * policy: the close window after a finished run must not convert an
 * already-completed repeat into cancelled or timed out. */
func TestBenchmarkCloseWindowDoesNotConsumeCompletedRun(t *testing.T) {
	runner, c := newCloseTestRunner(t, func(_ *foxruntime.AgentSession, timeout time.Duration) error {
		time.Sleep(700 * time.Millisecond)
		_ = timeout
		return nil
	})
	runner.caseTimeout = func(*Case) time.Duration { return 500 * time.Millisecond }
	result, err := runner.RunRepeat(context.Background(), c, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(result.Workspace)
	if !result.Success || result.Status != ResultStatusCompleted {
		t.Fatalf("result = %#v, want the completed verdict after the deadline passed during close", result)
	}
}
