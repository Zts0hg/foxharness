package subagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type recordingRunner struct {
	request Request
	calls   int
	result  *Result
	err     error
	enforce bool
}

func TestIACHD003DelegatePassesFrozenParentCapabilitySnapshot(t *testing.T) {
	runner := &recordingRunner{enforce: true, result: &Result{Status: OutcomeSucceeded}}
	tool := NewTool(runner, "parent-session")
	allowed := []string{"delegate_task", "read_file"}
	ctx := toolprotocol.WithCapabilities(context.Background(), allowed)
	allowed[1] = "write_file"
	if _, err := tool.Execute(ctx, json.RawMessage(`{"task":"inspect"}`)); err != nil {
		t.Fatal(err)
	}
	if len(runner.request.AllowedTools) != 2 || runner.request.AllowedTools[1] != "read_file" {
		t.Fatalf("child parent capability snapshot = %v", runner.request.AllowedTools)
	}
}

func (r *recordingRunner) Run(_ context.Context, request Request) (*Result, error) {
	r.calls++
	r.request = request
	return r.result, r.err
}

func (r *recordingRunner) PermissionEnforced() bool { return r.enforce }

func TestIACHD004DelegateUsesConsumerOwnedRunnerExactlyOnce(t *testing.T) {
	runner := &recordingRunner{enforce: true, result: &Result{SessionID: "child-session", Report: "done", Status: OutcomeSucceeded}}
	tool := NewTool(runner, "parent-session")
	ctx := toolprotocol.WithToolCall(tools.WithRunContext(context.Background(), "parent-session", "parent-run"), "delegate-call")
	result, err := tool.Execute(ctx, json.RawMessage(`{"task":"  inspect files  ","read_only":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if runner.request.ParentSessionID != "parent-session" || runner.request.ParentRunID != "parent-run" ||
		runner.request.DelegationID != "delegate-call" || runner.request.Task != "inspect files" || runner.request.ReadOnly || runner.request.Depth != 1 {
		t.Fatalf("runner request = %#v", runner.request)
	}
	if result != "Subagent Session: child-session\n\nReport:\ndone" {
		t.Fatalf("delegate output = %q", result)
	}
}
