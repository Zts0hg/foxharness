package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlatformReadOnlyBashContainsWritesReadsAndEnvironment(t *testing.T) {
	t.Setenv("FOX_READ_ONLY_SECRET", "must-not-leak")
	workDir := t.TempDir()
	outsideDir := t.TempDir()
	inside := filepath.Join(workDir, "inside.txt")
	outside := filepath.Join(outsideDir, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := newPlatformReadOnlyBashRunner()
	base := readOnlyBashRequest{
		WorkDir:       workDir,
		ReadableRoots: []string{workDir},
		Timeout:       time.Second,
	}
	safeRequest := base
	safeRequest.Command = "pwd"
	safeResult := runner.Run(context.Background(), safeRequest)
	switch {
	case safeResult.Err == nil:
		resolvedWorkDir, err := filepath.EvalSymlinks(workDir)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(safeResult.Output) != resolvedWorkDir {
			t.Fatalf("platform sandbox pwd = %q, want %q", safeResult.Output, resolvedWorkDir)
		}
		t.Log("platform read-only sandbox established")
	case errors.Is(safeResult.Err, ErrReadOnlyBashSandboxUnavailable):
		t.Logf("platform read-only sandbox unavailable and failed closed: %v", safeResult.Err)
	default:
		t.Fatalf("platform sandbox failed after establishment: %#v", safeResult)
	}

	writeRequest := base
	writeRequest.Command = fmt.Sprintf("printf changed > %q", inside)
	writeResult := runner.Run(context.Background(), writeRequest)
	if writeResult.Err == nil {
		t.Fatalf("platform sandbox accepted workspace mutation: %#v", writeResult)
	}
	if _, err := os.Stat(inside); !os.IsNotExist(err) {
		t.Fatalf("platform sandbox created %q: %v", inside, err)
	}

	readRequest := base
	readRequest.Command = fmt.Sprintf("cat %q", outside)
	readResult := runner.Run(context.Background(), readRequest)
	if readResult.Err == nil || strings.Contains(readResult.Output, "outside-secret") {
		t.Fatalf("platform sandbox exposed outside root: %#v", readResult)
	}

	environmentRequest := base
	environmentRequest.Command = "/usr/bin/env"
	environmentResult := runner.Run(context.Background(), environmentRequest)
	if strings.Contains(environmentResult.Output, "FOX_READ_ONLY_SECRET") || strings.Contains(environmentResult.Output, "must-not-leak") {
		t.Fatalf("platform sandbox inherited parent environment: %#v", environmentResult)
	}
}

func TestReadOnlyBashReadsWorkspaceThroughPlatformOrFailsClosed(t *testing.T) {
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "fixture.txt"), []byte("workspace-data"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewReadOnlyBashTool(workDir).ExecuteResult(context.Background(), bashCommandArgs(t, "cat fixture.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Failed {
		if strings.Contains(result.Output, ErrReadOnlyBashSandboxUnavailable.Error()) {
			t.Logf("platform read-only sandbox unavailable and failed closed: %s", result.Output)
			return
		}
		t.Fatalf("platform read-only Bash failed after sandbox establishment: %#v", result)
	}
	if result.Output != "workspace-data" {
		t.Fatalf("platform read-only Bash output = %q, want workspace-data", result.Output)
	}
}
