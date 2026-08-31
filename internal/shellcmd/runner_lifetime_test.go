package shellcmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

/* TestRunNormalReturnLetsBackgroundSurvivorLive pins the baseline bash
 * lifetime: when a command returns normally the direct child is reaped and a
 * backgrounded survivor keeps running, so a resident service started with
 * `nohup ... &` outlives the tool call. */
func TestRunNormalReturnLetsBackgroundSurvivorLive(t *testing.T) {
	workDir := t.TempDir()
	survivor := workDir + "/survivor"
	result := Run(context.Background(), workDir,
		"nohup bash -c 'sleep 1; touch "+survivor+"' >/dev/null 2>&1 & echo launched", 30*time.Second)
	if result.Err != nil {
		t.Fatalf("Err = %v, want a normal return", result.Err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(survivor); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("background survivor never ran; the normal return killed the process group")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

/* TestRunCancellationStillKillsProcessTree pins the cancellation path: a
 * cancelled command loses the whole process group instead of leaking it. */
func TestRunCancellationStillKillsProcessTree(t *testing.T) {
	workDir := t.TempDir()
	marker := workDir + "/completed"
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	result := Run(ctx, workDir, "sleep 1; touch "+marker, time.Minute)
	/* The killed process's own exit error is the baseline's cancellation
	 * result: a signal death carries exit code -1, not the context error. */
	var exitErr *exec.ExitError
	if !errors.As(result.Err, &exitErr) || result.ExitCode != -1 {
		t.Fatalf("Err = %v/%d, want the killed process exit error with code -1", result.Err, result.ExitCode)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker stat = %v, want the cancelled command tree to be gone", err)
	}
}
