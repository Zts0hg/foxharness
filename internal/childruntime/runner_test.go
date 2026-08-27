package childruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
)

type captureProvider struct {
	messages []schema.Message
	tools    []schema.ToolDefinition
}

func (p *captureProvider) Generate(_ context.Context, messages []schema.Message, tools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.messages = append([]schema.Message(nil), messages...)
	p.tools = append([]schema.ToolDefinition(nil), tools...)
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "child report"}}, nil
}

func (*captureProvider) ProviderProtocol() string { return "openai" }
func (*captureProvider) ModelName() string        { return "child-model" }

func TestNewTreatsTypedNilProviderAsAbsent(t *testing.T) {
	var model *provider.OpenAIProvider
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("New() panicked for typed-nil provider metadata: %v", recovered)
		}
	}()
	runner := New(Config{Provider: model})
	if runner == nil {
		t.Fatal("New() = nil, want runner with absent provider metadata")
	}
	if runner.config.ProviderProtocol != "" || runner.config.Model != "" {
		t.Fatalf("typed-nil provider metadata = protocol %q/model %q, want absent metadata", runner.config.ProviderProtocol, runner.config.Model)
	}
}

func TestRunnerExecutesThroughRuntimeChildProfile(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	model := &captureProvider{}
	coordinator := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeFullAccess, true),
	})
	runner := New(Config{
		Provider: model, WorkDir: workDir, ParentProfile: TUIInteractive,
		Permission: coordinator,
	})
	result, err := runner.Run(context.Background(), subagent.Request{
		ParentSessionID: "parent-session", ParentRunID: "parent-run", DelegationID: "tool-call",
		Task: "inspect", ReadOnly: true, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != subagent.OutcomeSucceeded || result.Report != "child report" || result.SessionID == "" || result.RunID == "" {
		t.Fatalf("child outcome = %#v", result)
	}
	if len(model.tools) != 2 || model.tools[0].Name != "bash" || model.tools[1].Name != "read_file" {
		t.Fatalf("child tools = %#v", model.tools)
	}
	child, err := session.NewFileStore(workDir).Open(session.ID(result.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentSessionID != "parent-session" || child.ParentRunID != "parent-run" || child.DelegationID != "tool-call" {
		t.Fatalf("child lineage = %#v", child)
	}
}

func TestRunnerStoresChildSessionUnderFrozenHomeDir(t *testing.T) {
	ambientHome := t.TempDir()
	configuredHome := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", ambientHome)
	runner := New(Config{
		Provider: &captureProvider{}, WorkDir: workDir, HomeDir: configuredHome,
		ParentProfile: CLIExec,
	})

	result, err := runner.Run(context.Background(), subagent.Request{
		ParentSessionID: "parent-session", ParentRunID: "parent-run", DelegationID: "tool-call",
		Task: "inspect", ReadOnly: true, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewFileStoreWithHome(workDir, configuredHome).Open(session.ID(result.SessionID)); err != nil {
		t.Fatalf("open child session from configured home: %v", err)
	}
	if _, err := session.NewFileStoreWithHome(workDir, ambientHome).Open(session.ID(result.SessionID)); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("open child session from ambient home error = %v, want ErrNotFound", err)
	}
}

func TestCLIExecRunnerExecutesWithoutPermissionCoordinator(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	model := &captureProvider{}
	runner := New(Config{
		Provider: model, WorkDir: workDir, ParentProfile: CLIExec,
	})

	result, err := runner.Run(context.Background(), subagent.Request{
		ParentSessionID: "parent-session", ParentRunID: "parent-run", DelegationID: "tool-call",
		Task: "inspect", ReadOnly: true, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != subagent.OutcomeSucceeded || result.Report != "child report" {
		t.Fatalf("child outcome = %#v", result)
	}
	if len(model.tools) != 2 || model.tools[0].Name != "bash" || model.tools[1].Name != "read_file" {
		t.Fatalf("child tools = %#v", model.tools)
	}
}

func TestRunnerPreservesExplicitEmptyParentTools(t *testing.T) {
	homeDir := t.TempDir()
	workDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	model := &captureProvider{}
	runner := New(Config{
		Provider: model, WorkDir: workDir, ParentProfile: CLIExec,
		ParentTools: []string{},
	})

	result, err := runner.Run(context.Background(), subagent.Request{
		ParentSessionID: "parent-session", ParentRunID: "parent-run", DelegationID: "tool-call",
		Task: "inspect", ReadOnly: false, Depth: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != subagent.OutcomeSucceeded || result.Report != "child report" {
		t.Fatalf("child outcome = %#v", result)
	}
	if len(model.tools) != 0 {
		t.Fatalf("child tools = %#v, want no tools from explicit empty parent ceiling", model.tools)
	}
}

func TestDelegationPolicyAllowsOnlyProfilesWithSatisfiedPermissionSemantics(t *testing.T) {
	for _, test := range []struct {
		profile ParentProfile
		want    bool
	}{
		{profile: CLIExec, want: true},
		{profile: AutodevPipeline, want: true},
		{profile: TUIInteractive, want: false},
		{profile: FeishuRemote, want: false},
		{profile: AgentOpsTask, want: false},
	} {
		runner := New(Config{ParentProfile: test.profile})
		if got := runner.DelegationAllowed(); got != test.want {
			t.Fatalf("profile %s DelegationAllowed() = %t, want %t", test.profile, got, test.want)
		}
	}
}

func TestRunnerNormalizesRejectedRequestBeforeAgentResolution(t *testing.T) {
	workDir := t.TempDir()
	runner := New(Config{WorkDir: workDir})

	result, err := runner.Run(context.Background(), subagent.Request{
		ParentSessionID: "parent-session",
		ParentRunID:     "parent-run",
		DelegationID:    "tool-call",
		Agent:           subagent.AgentID("unknown-agent"),
		Task:            "inspect",
	})

	if err == nil {
		t.Fatal("unknown agent error = nil")
	}
	if result == nil || result.Status != subagent.OutcomeRejected || result.Depth != 1 {
		t.Fatalf("rejected outcome = %#v, want normalized depth-one rejection", result)
	}
	if result.ParentSessionID != "parent-session" || result.ParentRunID != "parent-run" || result.DelegationID != "tool-call" {
		t.Fatalf("rejected lineage = %#v", result)
	}
	if _, openErr := session.NewFileStore(workDir).Open(session.ID(result.SessionID)); !errors.Is(openErr, session.ErrNotFound) {
		t.Fatalf("rejected child session lookup error = %v, want ErrNotFound", openErr)
	}
}
