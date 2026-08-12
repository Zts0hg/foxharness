package subagent

import (
	"context"
	"testing"

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

type childDefinitionCaptureProvider struct {
	toolNames []string
}

func (p *childDefinitionCaptureProvider) Generate(_ context.Context, _ []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.toolNames = p.toolNames[:0]
	for _, definition := range definitions {
		p.toolNames = append(p.toolNames, definition.Name)
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}
