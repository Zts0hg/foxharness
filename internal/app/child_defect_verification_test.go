package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/subagent"
)

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
