package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type failingForkChildProvider struct {
	calls int
	err   error
}

func (p *failingForkChildProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.calls == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "committed fork partial",
			ToolCalls: []schema.ToolCall{{
				ID:        "missing",
				Name:      "missing_tool",
				Arguments: []byte(`{}`),
			}},
		}}, nil
	}
	return nil, p.err
}

type forkChildCaptureProvider struct {
	mu       sync.Mutex
	messages []schema.Message
	tools    []schema.ToolDefinition
}

func (p *forkChildCaptureProvider) Generate(_ context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.messages = append([]schema.Message(nil), messages...)
	p.tools = append([]schema.ToolDefinition(nil), availableTools...)
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "fork report"}}, nil
}

func (*forkChildCaptureProvider) ProviderProtocol() string { return "openai" }
func (*forkChildCaptureProvider) ModelName() string        { return "fork-model" }

func TestDVCHD005ForkAdapterPropagatesGeneralPurposeAgentToChildInvocation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("project-child-instruction"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider := &forkChildCaptureProvider{}
	runner := &subagentForkRunner{
		getManager: func() *subagent.Manager { return subagent.NewManager(provider, workDir) },
		getSession: func() string { return "parent-session" },
	}

	report, err := runner.Run(context.Background(), "processed fork task", "general-purpose", []string{"read_file"})
	if err != nil {
		t.Fatal(err)
	}
	if report != "fork report" {
		t.Fatalf("fork report = %q, want current child report", report)
	}

	provider.mu.Lock()
	messages := append([]schema.Message(nil), provider.messages...)
	definitions := append([]schema.ToolDefinition(nil), provider.tools...)
	provider.mu.Unlock()
	var visible strings.Builder
	for _, message := range messages {
		visible.WriteString(message.Content)
		visible.WriteByte('\n')
	}
	for _, want := range []string{"processed fork task", "project-child-instruction", "parent-session", "Agent: general-purpose"} {
		if !strings.Contains(visible.String(), want) {
			t.Fatalf("child invocation missing %q:\n%s", want, visible.String())
		}
	}
	if len(definitions) != 1 || definitions[0].Name != "read_file" {
		t.Fatalf("fork tool ceiling = %+v, want only read_file", definitions)
	}
}

func TestDVCHD005ForkAdapterRejectsUnknownAgentBeforeChildInvocation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	provider := &forkChildCaptureProvider{}
	runner := &subagentForkRunner{
		getManager: func() *subagent.Manager { return subagent.NewManager(provider, t.TempDir()) },
		getSession: func() string { return "parent-session" },
	}

	report, err := runner.Run(context.Background(), "processed fork task", "selected-agent-marker", []string{"read_file"})
	if err == nil {
		t.Fatalf("unknown fork agent returned report %q, want explicit rejection", report)
	}
	if !strings.Contains(err.Error(), `unknown agent "selected-agent-marker"`) {
		t.Fatalf("unknown fork agent error = %q, want explicit identity", err)
	}

	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.messages) != 0 {
		t.Fatalf("unknown fork agent reached child provider with %d messages", len(provider.messages))
	}
}

func TestDVCHD006ForkAdapterRetainsPartialOutcomeAndTerminalError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	terminalErr := errors.New("fork provider failed")
	provider := &failingForkChildProvider{err: terminalErr}
	runner := &subagentForkRunner{
		getManager: func() *subagent.Manager { return subagent.NewManager(provider, t.TempDir()).WithMaxTurns(3) },
		getSession: func() string { return "parent-session" },
	}

	output, err := runner.Run(context.Background(), "fork failure", "general-purpose", []string{"read_file"})
	var outcomeErr *subagent.OutcomeError
	if !errors.As(err, &outcomeErr) || !errors.Is(err, terminalErr) {
		t.Fatalf("fork error = %T/%v, want typed outcome wrapping provider cause", err, err)
	}
	if outcomeErr.Outcome == nil || outcomeErr.Outcome.Status != subagent.OutcomeFailed {
		t.Fatalf("fork typed outcome = %#v", outcomeErr.Outcome)
	}
	for _, want := range []string{"Status: failed", "Partial Report:", "committed fork partial"} {
		if !strings.Contains(output, want) {
			t.Fatalf("fork output missing %q:\n%s", want, output)
		}
	}
}

func TestIACHD005ForkAdapterCarriesLiveParentInvocationLineage(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	workDir := t.TempDir()
	provider := &forkChildCaptureProvider{}
	manager := subagent.NewManager(provider, workDir)
	runner := &subagentForkRunner{
		getManager: func() *subagent.Manager { return manager },
		getSession: func() string { return "parent-session" },
	}
	registry := tools.NewRegistry()
	registry.Register(forkInvocationTool{runner: runner})
	ctx := tools.WithRunContext(context.Background(), "parent-session", "parent-run")
	result := registry.Execute(ctx, schema.ToolCall{ID: "fork-tool-call", Name: "fork_test", Arguments: json.RawMessage(`{}`)})
	if result.IsError {
		t.Fatalf("fork adapter execution = %#v", result)
	}
	child, err := session.NewManagerWithHome(workDir, homeDir).Latest(session.LookupOptions{Source: session.SOURCESubagent})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentRunID != "parent-run" || child.DelegationID != "fork-tool-call" {
		t.Fatalf("fork child lineage = %#v", child)
	}
}

type forkInvocationTool struct {
	runner *subagentForkRunner
}

func (forkInvocationTool) Name() string { return "fork_test" }

func (forkInvocationTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{Name: "fork_test", InputSchema: map[string]interface{}{"type": "object"}}
}

func (t forkInvocationTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	return t.runner.Run(ctx, "processed fork task", "general-purpose", []string{"read_file"})
}
