package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type bashCommandRunnerStub struct {
	calls int
}

func TestBashProcessSupervisorCancellationTerminatesAndReapsActiveGroup(t *testing.T) {
	workDir := t.TempDir()
	started := filepath.Join(workDir, "started")
	supervisor := NewBashProcessSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan BashCommandResult, 1)
	go func() {
		done <- supervisor.Run(ctx, workDir, "trap '' TERM; touch started; while :; do sleep 1; done", 10*time.Second)
	}()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(started); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("cancelled supervisor error = %v, want context.Canceled", result.Err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled supervised process did not return")
	}
	if active := supervisor.activeCount(); active != 0 {
		t.Fatalf("active supervised processes = %d after cancellation", active)
	}
	if err := supervisor.Cleanup(context.Background()); err != nil {
		t.Fatalf("post-cancellation cleanup error = %v", err)
	}
}

func (r *bashCommandRunnerStub) Run(context.Context, string, string, time.Duration) BashCommandResult {
	r.calls++
	return BashCommandResult{Output: "runner reached"}
}

func TestSupervisedBashRejectsBackgroundAndDetachBeforeRunner(t *testing.T) {
	runner := &bashCommandRunnerStub{}
	tool := NewSupervisedBashTool(t.TempDir(), runner)
	commands := []string{
		"sleep 1 &",
		"coproc sleep 1",
		"nohup sleep 1",
		"setsid sleep 1",
		"disown",
		"bash -c 'sleep 1 &'",
		"cat <(sleep 1)",
	}
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			result, err := tool.ExecuteResult(context.Background(), bashCommandArgs(t, command))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Failed {
				t.Fatalf("supervised Bash accepted asynchronous command %q: %#v", command, result)
			}
		})
	}
	if runner.calls != 0 {
		t.Fatalf("rejected asynchronous commands reached runner %d times", runner.calls)
	}
}

func TestBashProcessSupervisorRegistersBeforeStartAndCleansAfterWait(t *testing.T) {
	supervisor := NewBashProcessSupervisor()
	registered := false
	supervisor.beforeStart = func(active int) {
		registered = active == 1
	}
	result := supervisor.Run(context.Background(), t.TempDir(), "printf supervised", time.Second)
	if result.Err != nil || result.Output != "supervised" {
		t.Fatalf("supervised command output/error = %q/%v", result.Output, result.Err)
	}
	if !registered {
		t.Fatal("command was not registered before process start")
	}
	if active := supervisor.activeCount(); active != 0 {
		t.Fatalf("active supervised processes = %d after wait, want 0", active)
	}
	if err := supervisor.Cleanup(context.Background()); err != nil {
		t.Fatalf("idle cleanup error = %v", err)
	}
}
