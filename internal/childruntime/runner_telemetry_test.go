package childruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
)

/* TestRunnerWritesChildRunMetrics pins the baseline child telemetry: a child
 * run records the same run-level metrics artifact the parent compositions
 * write, so frozen-snapshot consumers can compare child and parent runs. */
func TestRunnerWritesChildRunMetrics(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	runner := New(Config{
		Provider: &captureProvider{}, WorkDir: workDir, HomeDir: homeDir, ParentProfile: CLIExec,
	})
	result, err := runner.Run(context.Background(), subagent.Request{
		ParentSessionID: "parent-session", ParentRunID: "parent-run", DelegationID: "tool-call",
		Task: "inspect", ReadOnly: true, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := session.NewFileStoreWithHome(workDir, homeDir).Open(session.ID(result.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(child.RootDir, "runs", "*", "metrics.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("child metrics artifacts = %v, want one run summary", matches)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"run_summary"`) {
		t.Fatalf("child metrics = %q, want a run summary record", content)
	}
}
