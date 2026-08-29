package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestBashProcessSupervisorCancellationPreventsDescendantSideEffects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("process-group cancellation requires Unix")
	}
	workDir := t.TempDir()
	started := filepath.Join(workDir, "started")
	leaked := filepath.Join(workDir, "leaked")
	supervisor := NewBashProcessSupervisor()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan BashCommandResult, 1)
	go func() {
		command := "(trap '' TERM; touch started; sleep 0.1; touch leaked) & wait"
		done <- supervisor.Run(ctx, workDir, command, 10*time.Second)
	}()
	waitForSupervisorFile(t, started, time.Second)
	cancel()
	select {
	case result := <-done:
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("cancelled supervisor error = %v, want context.Canceled", result.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled supervised process did not return")
	}
	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(leaked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled descendant produced a late side effect: %v", err)
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
		"echo sleep 1 | at now",
		"batch < script.sh",
		"systemd-run --collect sleep 1",
		"launchctl submit -l label -- sleep 1",
		"launchctl load plists/com.example.payload.plist",
		"crontab -",
		"schtasks /create /tn demo /tr calc /sc once",
		"open -a Terminal",
		`"at" now -f script.sh`,
		`'launchctl' submit -l label -- sleep 1`,
		"/usr/bin/at now -f script.sh",
		"/bin/bash -c 'sleep 1 &'",
		"/usr/bin/open -a Terminal",
		"/usr/bin/crontab -",
		`\at now -f script.sh`,
		`\setsid sleep 1`,
		`\nohup sleep 1`,
		`/usr/bin/\at now`,
		`{a,}t now -f script.sh`,
		`a?t now`,
		`[a]t now`,
		"builtin eval 'payload'",
		"trap 'at now' EXIT",
		"nice at now",
		"timeout 5 at now",
		"stdbuf -o0 at now",
		"sudo at now",
		"xargs at now",
		"find . -name x -exec at now ;",
		"watch -n1 at now",
		`find . -e"x"ec at now ;`,
		"find . -exec nice at now ;",
		"taskset -c 0 setsid sleep 1000",
		"ionice -c 2 at now",
		"chrt -r 1 setsid sleep 1000",
		"script -qc 'setsid sleep 1000' /dev/null",
		"python3 -c 'import os;os.setsid()'",
		"perl -e 'exec \"setsid\", \"sleep\"'",
		"node -e 'require(\"child_process\").spawn(\"at\",[\"now\"],{detached:true})'",
		"xargs at now",
		"printf 'at\\0now\\0' | xargs -0",
		"make deploy",
		`find . -maxdepth 1 -exec true \; -exec setsid sleep 5 {} \;`,
		"tcsh -c 'sleep 1'",
		"busybox setsid sleep 5",
		"gmake deploy",
		"shopt -s expand_aliases",
		"python3.13t -c 'print(1)'",
		"luajit script.lua",
		"SUDO at now",
		"NICE python3 -c 'print(1)'",
		"FIND . -exec sh -c 'setsid sleep 5' ;",
		"find . \"$(echo -exec)\" sh -c 'setsid sleep 5' ;",
		"ksh93 script.ksh",
		"zsh-5.9 -c 'sleep 1'",
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

func waitForSupervisorFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("file %q was not created before timeout", path)
}
