package permission

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestCoordinatorRoutesAskModeToUserAndStoresSessionGrant(t *testing.T) {
	state := NewState(ModeAsk, false)
	approver := &fakeApprover{decision: UserDecision{Kind: UserAllowSession}}
	coordinator := NewCoordinator(Config{
		State: state, Workspace: t.TempDir(), CWD: t.TempDir(), Source: SourceMain, Approver: approver,
	})
	call := toolCall("bash", map[string]string{"command": "go test ./..."})

	if err := coordinator.Authorize(context.Background(), call); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if approver.calls != 1 {
		t.Fatalf("approver calls = %d, want 1", approver.calls)
	}
	if err := coordinator.Authorize(context.Background(), call); err != nil {
		t.Fatalf("Authorize() with grant error = %v", err)
	}
	if approver.calls != 1 {
		t.Fatalf("approver calls after grant = %d, want 1", approver.calls)
	}
}

func TestRegistryDecoratorDeniesBeforeExecuteAndDisablesParallel(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register(fakeTool{name: "danger"})
	coordinator := NewCoordinator(Config{
		State:     NewState(ModeAsk, false),
		Workspace: t.TempDir(),
		CWD:       t.TempDir(),
		Approver:  &fakeApprover{decision: UserDecision{Kind: UserDeny}},
	})
	wrapped := DecorateRegistry(registry, coordinator)
	if wrapped.IsParallelSafe("danger") {
		t.Fatal("permission registry must not be parallel-safe")
	}
	result := wrapped.Execute(context.Background(), schema.ToolCall{ID: "1", Name: "danger", Arguments: json.RawMessage(`{}`)})
	if !result.IsError {
		t.Fatalf("Execute() IsError = false, want true")
	}
}

func TestCoordinatorPassesInjectedEvidenceToReviewer(t *testing.T) {
	reviewer := &capturingReviewer{result: ReviewResult{
		Decision:          ReviewApprove,
		Risk:              RiskLow,
		UserAuthorization: AuthorizationMedium,
		Rationale:         "scoped",
	}}
	coordinator := NewCoordinator(Config{
		State:     NewState(ModeApprove, false),
		Workspace: t.TempDir(),
		CWD:       t.TempDir(),
		Reviewer:  reviewer,
		Evidence: func(request Request) Evidence {
			return Evidence{Text: "trusted current user task"}
		},
	})

	if err := coordinator.Authorize(context.Background(), toolCall("bash", map[string]string{"command": "go test ./..."})); err != nil {
		t.Fatalf("Authorize() error = %v", err)
	}
	if reviewer.evidence.Text != "trusted current user task" {
		t.Fatalf("review evidence = %q, want injected task context", reviewer.evidence.Text)
	}
}

type fakeApprover struct {
	decision UserDecision
	calls    int
}

func (a *fakeApprover) Approve(ctx context.Context, request ApprovalRequest) (UserDecision, error) {
	a.calls++
	return a.decision, nil
}

type capturingReviewer struct {
	result   ReviewResult
	evidence Evidence
}

func (r *capturingReviewer) Review(ctx context.Context, request Request, evidence Evidence) (ReviewResult, error) {
	r.evidence = evidence
	return r.result, nil
}

type fakeTool struct{ name string }

func (t fakeTool) Name() string { return t.name }
func (t fakeTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: t.name, InputSchema: map[string]any{"type": "object"}}
}
func (t fakeTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return "ok", nil
}
func (t fakeTool) ParallelSafe() bool { return true }
