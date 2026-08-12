package subagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestPFCHD002ActiveRequestFreezesCallerToolAllowlist(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	provider := &childDefinitionCaptureProvider{}
	manager := NewManager(provider, workDir)
	createStarted := make(chan struct{})
	createRelease := make(chan struct{})
	manager.createSession = func(options session.CreateOptions) (*session.Session, error) {
		close(createStarted)
		<-createRelease
		return session.NewManagerWithHome(workDir, home).Create(options)
	}
	allowed := []string{"read_file"}
	resultCh := make(chan *Result, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := manager.Run(context.Background(), Request{
			ParentSessionID: "parent",
			Task:            "inspect files",
			ReadOnly:        true,
			AllowedTools:    allowed,
		})
		resultCh <- result
		errCh <- err
	}()
	<-createStarted
	allowed[0] = "write_file"
	close(createRelease)
	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Status != OutcomeSucceeded {
		t.Fatalf("child result = %#v", result)
	}
	if got := provider.toolNames; len(got) != 1 || got[0] != "read_file" {
		t.Fatalf("active child tool snapshot = %v, want frozen [read_file]", got)
	}
}

func TestPFCHD006ConfiguredTurnBudgetCanOnlyNarrowProfileCeiling(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "minimum narrowing", in: 1, want: 1},
		{name: "exact ceiling", in: DefaultMaxTurns, want: DefaultMaxTurns},
		{name: "above ceiling", in: DefaultMaxTurns + 1, want: DefaultMaxTurns},
		{name: "zero", in: 0, want: DefaultMaxTurns},
		{name: "negative", in: -1, want: DefaultMaxTurns},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := NewManager(&finalReportProvider{}, t.TempDir()).WithMaxTurns(test.in)
			if manager.maxTurns != test.want {
				t.Fatalf("WithMaxTurns(%d) resolved %d, want %d", test.in, manager.maxTurns, test.want)
			}
		})
	}
}

func TestPFCHD011DepthGateRejectsNestedRunBeforeCapacityOrSession(t *testing.T) {
	createCalled := false
	manager := NewManager(&finalReportProvider{}, t.TempDir())
	manager.createSession = func(session.CreateOptions) (*session.Session, error) {
		createCalled = true
		return nil, nil
	}
	result, err := manager.Run(context.Background(), Request{
		ParentSessionID: "already-child",
		Task:            "attempt nested child",
		ReadOnly:        true,
		Depth:           2,
	})
	if err == nil {
		t.Fatalf("Run() error = nil, want nested depth rejection; result=%#v", result)
	}
	if createCalled {
		t.Fatal("nested depth rejection created child session capacity")
	}
	if result == nil || result.Status != OutcomeRejected || result.SessionID != "" || result.RunID != "" {
		t.Fatalf("nested depth outcome = %#v", result)
	}
}

func TestPFCHD014PermissionEvidenceCorrelatesCompleteChildLineage(t *testing.T) {
	workDir := t.TempDir()
	homeDir := t.TempDir()
	reviewer := &recordingPermissionReviewer{}
	coordinator := permission.NewCoordinator(permission.Config{
		State:     permission.NewState(permission.ModeApprove, false),
		Workspace: workDir,
		CWD:       workDir,
		Reviewer:  reviewer,
	})
	manager := NewManager(&childPermissionCallProvider{}, workDir).
		WithPermission(coordinator).
		WithParentEvidence(func(request permission.Request) permission.Evidence {
			return permission.BuildEvidence([]schema.Message{{Role: schema.RoleUser, Content: "trusted parent request"}}, nil, request)
		})
	manager.homeDir = homeDir
	manager.createSession = session.NewManagerWithHome(workDir, homeDir).Create

	result, err := manager.Run(context.Background(), Request{
		ParentSessionID: "parent-session",
		ParentRunID:     "parent-run",
		DelegationID:    "delegate-call",
		Task:            "inspect the workspace",
		ReadOnly:        false,
		Depth:           1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ParentSessionID != "parent-session" || result.ParentRunID != "parent-run" || result.DelegationID != "delegate-call" {
		t.Fatalf("terminal lineage = %#v", result)
	}
	wantCorrelation := permission.EvidenceCorrelation{
		ParentSessionID: "parent-session",
		ParentRunID:     "parent-run",
		ChildSessionID:  result.SessionID,
		ChildRunID:      result.RunID,
		DelegationID:    "delegate-call",
		ToolCallID:      "child-tool-call",
	}
	if reviewer.evidence.Correlation != wantCorrelation {
		t.Fatalf("permission correlation = %#v, want %#v", reviewer.evidence.Correlation, wantCorrelation)
	}
	if reviewer.result.Decision != permission.ReviewApprove || reviewer.result.Risk != permission.RiskLow {
		t.Fatalf("terminal permission result = %#v", reviewer.result)
	}
	stored, openErr := session.NewManagerWithHome(workDir, homeDir).Open(result.SessionID)
	if openErr != nil {
		t.Fatal(openErr)
	}
	if stored.ParentSessionID != "parent-session" || stored.ParentRunID != "parent-run" || stored.DelegationID != "delegate-call" {
		t.Fatalf("persisted child lineage = %#v", stored)
	}
}

type childDefinitionCaptureProvider struct {
	toolNames []string
}

type childPermissionCallProvider struct {
	calls int
}

func (p *childPermissionCallProvider) Generate(_ context.Context, _ []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.calls == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "child-tool-call",
				Name:      "bash",
				Arguments: json.RawMessage(`{"command":"true"}`),
			}},
		}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *childDefinitionCaptureProvider) Generate(_ context.Context, _ []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.toolNames = p.toolNames[:0]
	for _, definition := range definitions {
		p.toolNames = append(p.toolNames, definition.Name)
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}
