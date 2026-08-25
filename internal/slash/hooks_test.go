package slash

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExecuteHooks_NoHooksNoop(t *testing.T) {
	if err := ExecuteHooks(context.Background(), nil, "", 5*time.Second); err != nil {
		t.Errorf("unexpected err for nil hooks: %v", err)
	}
}

func TestExecuteHooks_BeforeRuns(t *testing.T) {
	wd := t.TempDir()
	marker := filepath.Join(wd, "before.touched")
	hooks := &FrontmatterHooks{Before: "touch " + marker}
	if err := ExecuteHooks(context.Background(), hooks, wd, 5*time.Second); err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("before hook did not run: %v", err)
	}
}

func TestExecuteAfterHook_Runs(t *testing.T) {
	wd := t.TempDir()
	marker := filepath.Join(wd, "after.touched")
	hooks := &FrontmatterHooks{After: "touch " + marker}
	if err := ExecuteAfterHook(context.Background(), hooks, wd, 5*time.Second); err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("after hook did not run: %v", err)
	}
}

func TestExecuteHooks_FailureNonBlocking(t *testing.T) {
	// Non-zero exit should NOT propagate as an error; the executor must
	// proceed even when a hook fails.
	hooks := &FrontmatterHooks{Before: "false"}
	if err := ExecuteHooks(context.Background(), hooks, "", 5*time.Second); err != nil {
		t.Errorf("expected non-blocking failure, got err: %v", err)
	}
}

func TestExecuteHooks_NilHookFields(t *testing.T) {
	hooks := &FrontmatterHooks{} // empty before/after
	if err := ExecuteHooks(context.Background(), hooks, "", 5*time.Second); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
	if err := ExecuteAfterHook(context.Background(), hooks, "", 5*time.Second); err != nil {
		t.Errorf("unexpected err: %v", err)
	}
}

func TestExecuteHooks_Timeout(t *testing.T) {
	hooks := &FrontmatterHooks{Before: "sleep 5"}
	if err := ExecuteHooks(context.Background(), hooks, "", 50*time.Millisecond); err != nil {
		t.Errorf("timeout should be non-blocking, got err: %v", err)
	}
}

func TestExecuteHooksCancellationKillsDescendants(t *testing.T) {
	workDir := t.TempDir()
	readyPath := filepath.Join(workDir, "ready")
	leakedPath := filepath.Join(workDir, "leaked")
	hooks := &FrontmatterHooks{Before: fmt.Sprintf(
		"(printf ready > %q; sleep 0.5; printf leaked > %q) & wait",
		readyPath,
		leakedPath,
	)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = ExecuteHooks(ctx, hooks, workDir, 5*time.Second)
		close(done)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("hook descendant did not reach readiness barrier")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled hook did not return")
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := os.Stat(leakedPath); !os.IsNotExist(err) {
		t.Fatalf("cancelled hook descendant side effect survived: %v", err)
	}
}
