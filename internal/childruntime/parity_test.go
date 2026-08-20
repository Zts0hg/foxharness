package childruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/automemory"
	legacycontext "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type parityProvider struct {
	messages []schema.Message
	tools    []schema.ToolDefinition
}

func (p *parityProvider) Generate(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.messages = append([]schema.Message(nil), messages...)
	p.tools = append([]schema.ToolDefinition(nil), definitions...)
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "parity report"}}, nil
}

func (*parityProvider) ProviderProtocol() string { return "openai" }
func (*parityProvider) ModelName() string        { return "parity-model" }

type childParityObservation struct {
	Status              subagent.OutcomeStatus
	Report              string
	ToolNames           []string
	HasTask             bool
	HasAgent            bool
	HasProjectDirective bool
	ParentSessionID     string
	ParentRunID         string
	DelegationID        string
	PersistedFinal      string
}

func TestM15LegacyAndRuntimeChildAdaptersPreserveBehavior(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	legacyDir := t.TempDir()
	targetDir := t.TempDir()
	for _, workDir := range []string{legacyDir, targetDir} {
		if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("PARITY_PROJECT_DIRECTIVE"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	request := subagent.Request{
		ParentSessionID: "parent-session", ParentRunID: "parent-run", DelegationID: "tool-call",
		Task: "inspect parity fixture", ReadOnly: true, Agent: subagent.AgentGeneralPurpose, Depth: 1,
	}

	legacy := observeLegacyChild(t, homeDir, legacyDir, request)
	target := observeRuntimeChild(t, targetDir, request)
	if !reflect.DeepEqual(target, legacy) {
		t.Fatalf("target child observation = %#v\nlegacy child observation = %#v", target, legacy)
	}
}

func observeLegacyChild(t *testing.T, homeDir, workDir string, request subagent.Request) childParityObservation {
	t.Helper()
	model := &parityProvider{}
	store := session.NewManagerWithHome(workDir, homeDir)
	child, err := store.Create(session.CreateOptions{
		Source: session.SOURCESubagent, WorkDir: workDir,
		UserID:          "subagent-of-" + request.ParentSessionID,
		ParentSessionID: session.ID(request.ParentSessionID), ParentRunID: session.RunID(request.ParentRunID),
		DelegationID: request.DelegationID, Agent: string(request.Agent),
	})
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewReadOnlyBashTool(workDir))
	composer := legacycontext.NewComposer(workDir).
		WithReadOnlyMemory(child.MemoryPath()).
		WithReadOnlyAutoMemory(automemory.NewStore(homeDir, workDir)).
		WithToolCapabilities([]string{"bash", "read_file"})
	prompt := fmt.Sprintf("Agent: general-purpose\n\n%s", request.Task)
	result, runErr := engine.NewLegacyEngine(model, registry, workDir, composer, engine.Config{
		MaxTurns: subagent.DefaultMaxTurns, ProviderProtocol: "openai", Model: "parity-model",
	}).Run(context.Background(), child, prompt)
	if runErr != nil {
		t.Fatal(runErr)
	}
	return parityObservation(t, child, model, subagent.OutcomeSucceeded, result.FinalMessage, request)
}

func observeRuntimeChild(t *testing.T, workDir string, request subagent.Request) childParityObservation {
	t.Helper()
	model := &parityProvider{}
	coordinator := permission.NewCoordinator(permission.Config{State: permission.NewState(permission.ModeFullAccess, true)})
	result, err := New(Config{
		Provider: model, WorkDir: workDir, ParentProfile: TUIInteractive, Permission: coordinator,
	}).Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	child, err := session.NewFileStore(workDir).Open(session.ID(result.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	return parityObservation(t, child, model, result.Status, result.Report, request)
}

func parityObservation(t *testing.T, child *session.StoredSession, model *parityProvider, status subagent.OutcomeStatus, report string, request subagent.Request) childParityObservation {
	t.Helper()
	var visible strings.Builder
	for _, message := range model.messages {
		visible.WriteString(message.Content)
		visible.WriteByte('\n')
	}
	names := make([]string, 0, len(model.tools))
	for _, definition := range model.tools {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	records, err := session.NewMessageLog(child).LoadMessages()
	if err != nil {
		t.Fatal(err)
	}
	final := ""
	for _, message := range records {
		if message.Role == schema.RoleAssistant && message.Content != "" {
			final = message.Content
		}
	}
	return childParityObservation{
		Status: status, Report: report, ToolNames: names,
		HasTask:             strings.Contains(visible.String(), request.Task),
		HasAgent:            strings.Contains(visible.String(), "Agent: general-purpose"),
		HasProjectDirective: strings.Contains(visible.String(), "PARITY_PROJECT_DIRECTIVE"),
		ParentSessionID:     string(child.ParentSessionID), ParentRunID: string(child.ParentRunID),
		DelegationID: child.DelegationID, PersistedFinal: final,
	}
}
