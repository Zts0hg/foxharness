package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// fakeRunnerAPI records adapter delegation.
type fakeRunnerAPI struct {
	workDir   string
	runPrompt string
	asker     tools.UserAsker
	model     string
	registry  *slash.Registry
	executor  *slash.Executor
}

func (f *fakeRunnerAPI) Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	f.runPrompt = prompt
	return &engine.RunResult{FinalMessage: "ran"}, nil
}
func (f *fakeRunnerAPI) SetUserAsker(asker tools.UserAsker) { f.asker = asker }
func (f *fakeRunnerAPI) SetModel(model string) error        { f.model = model; return nil }
func (f *fakeRunnerAPI) WorkDir() string                    { return f.workDir }
func (f *fakeRunnerAPI) SlashRegistry() *slash.Registry     { return f.registry }
func (f *fakeRunnerAPI) SlashExecutor() *slash.Executor     { return f.executor }
func (f *fakeRunnerAPI) SessionID() string                  { return "sess-1" }

type fakeAutodevAsker struct{}

func (fakeAutodevAsker) Ask(ctx context.Context, questions []tools.Question) ([]tools.Answer, error) {
	return nil, nil
}

func TestCoreRunnerAdapterDelegates(t *testing.T) {
	api := &fakeRunnerAPI{workDir: "/wt/item"}
	adapter := &coreRunnerAdapter{runner: api}
	var _ autodev.CoreRunner = adapter

	res, err := adapter.Run(context.Background(), "do it", nil)
	if err != nil || res.FinalMessage != "ran" {
		t.Fatalf("Run = (%v, %v), want delegation to the real runner", res, err)
	}
	if api.runPrompt != "do it" {
		t.Errorf("runner prompt = %q, want %q", api.runPrompt, "do it")
	}

	adapter.SetUserAsker(fakeAutodevAsker{})
	if api.asker == nil {
		t.Error("SetUserAsker did not reach the runner (REQ-013)")
	}
	if err := adapter.SetModel("glm-4.7"); err != nil || api.model != "glm-4.7" {
		t.Errorf("SetModel: err=%v model=%q, want delegated", err, api.model)
	}
	if adapter.WorkDir() != "/wt/item" {
		t.Errorf("WorkDir = %q, want delegated", adapter.WorkDir())
	}
}

func TestCoreRunnerAdapterStagePrompt(t *testing.T) {
	registry := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	registry.Register(&slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "codexspec:generate-spec",
		Source:  slash.SourceProject,
		Content: "Generate the spec.\n\n## User Input\n\n$ARGUMENTS",
	})
	api := &fakeRunnerAPI{
		registry: registry,
		executor: slash.NewExecutor(slash.WithWorkDir(t.TempDir())),
	}
	adapter := &coreRunnerAdapter{runner: api}

	prompt, err := adapter.StagePrompt(context.Background(), "codexspec:generate-spec", "build the thing")
	if err != nil {
		t.Fatalf("StagePrompt returned error: %v", err)
	}
	if !strings.Contains(prompt, "Generate the spec.") {
		t.Errorf("prompt = %q, want command body", prompt)
	}
	if !strings.Contains(prompt, "build the thing") {
		t.Errorf("prompt = %q, want $ARGUMENTS substituted", prompt)
	}
}

func TestCoreRunnerAdapterStagePromptUnknownCommand(t *testing.T) {
	api := &fakeRunnerAPI{
		registry: slash.NewRegistry(t.TempDir()).WithoutDiscovery(),
		executor: slash.NewExecutor(),
	}
	adapter := &coreRunnerAdapter{runner: api}

	if _, err := adapter.StagePrompt(context.Background(), "codexspec:nope", ""); err == nil {
		t.Fatal("StagePrompt returned nil error for an unknown command, want error")
	}
}

func TestCoreRunnerAdapterStagePromptHonorsContext(t *testing.T) {
	registry := slash.NewRegistry(t.TempDir()).WithoutDiscovery()
	// The embedded command transforms its text so executed output ("OK")
	// is distinguishable from the command line echoed inside error markers.
	registry.Register(&slash.Command{
		Type:    slash.CommandPrompt,
		Name:    "codexspec:shelly",
		Source:  slash.SourceProject,
		Content: "Before !`echo ok | tr a-z A-Z` after",
	})
	api := &fakeRunnerAPI{
		registry: registry,
		executor: slash.NewExecutor(slash.WithWorkDir(t.TempDir())),
	}
	adapter := &coreRunnerAdapter{runner: api}

	live, err := adapter.StagePrompt(context.Background(), "codexspec:shelly", "")
	if err != nil {
		t.Fatalf("StagePrompt returned error: %v", err)
	}
	if !strings.Contains(live, "OK") {
		t.Fatalf("prompt = %q, want embedded shell output with a live context", live)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	dead, err := adapter.StagePrompt(cancelled, "codexspec:shelly", "")
	if err == nil && strings.Contains(dead, "OK") {
		t.Errorf("prompt = %q, want the cancelled context to stop embedded shell execution (CODE-003)", dead)
	}
}

func TestResolveAutodevModelPrecedence(t *testing.T) {
	if got := resolveAutodevModel("cli-model", autodev.AutodevConfig{Model: "yml-model"}); got != "yml-model" {
		t.Errorf("model = %q, want autodev.yml to win", got)
	}
	if got := resolveAutodevModel("cli-model", autodev.AutodevConfig{}); got != "cli-model" {
		t.Errorf("model = %q, want CLI fallback", got)
	}
}

func TestBuildAutodevDepsSharesOneModel(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "test-key")
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".foxharness"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".foxharness", "autodev.yml"), []byte("model: shared-model\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	deps, err := buildAutodevDeps(context.Background(), CLIConfig{WorkDir: repoRoot, Model: "cli-model", Provider: "openai"}, autodev.NewTerminalReporter(os.Stderr))
	if err != nil {
		t.Fatalf("buildAutodevDeps returned error: %v", err)
	}

	if deps.Config.Model != "shared-model" {
		t.Errorf("deps.Config.Model = %q, want shared-model", deps.Config.Model)
	}
	engineerAgent, ok := deps.Engineer.(*autodev.ProviderEngineerAgent)
	if !ok {
		t.Fatalf("Engineer is %T, want *autodev.ProviderEngineerAgent", deps.Engineer)
	}
	if engineerAgent.Model() != deps.Config.Model {
		t.Errorf("engineer model = %q, core model = %q, want identical (TC-022, REQ-016)", engineerAgent.Model(), deps.Config.Model)
	}
	factory, ok := deps.CoreFactory.(*appCoreRunnerFactory)
	if !ok {
		t.Fatalf("CoreFactory is %T, want *appCoreRunnerFactory", deps.CoreFactory)
	}
	if factory.model != deps.Config.Model {
		t.Errorf("factory model = %q, want %q (TC-022)", factory.model, deps.Config.Model)
	}
}

func TestResolveEngineerPersona(t *testing.T) {
	repoRoot := t.TempDir()
	personaPath := filepath.Join(repoRoot, "persona.md")
	if err := os.WriteFile(personaPath, []byte("You are Margaret."), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveEngineerPersona(autodev.AutodevConfig{EngineerPromptFile: "persona.md"}, repoRoot)
	if err != nil {
		t.Fatalf("resolveEngineerPersona returned error: %v", err)
	}
	if got != "You are Margaret." {
		t.Errorf("persona = %q, want file contents (TC-015)", got)
	}

	got, err = resolveEngineerPersona(autodev.AutodevConfig{EngineerPrompt: "inline persona"}, repoRoot)
	if err != nil || got != "inline persona" {
		t.Errorf("persona = (%q, %v), want inline value", got, err)
	}

	got, err = resolveEngineerPersona(autodev.AutodevConfig{}, repoRoot)
	if err != nil || got != "" {
		t.Errorf("persona = (%q, %v), want empty so the default applies", got, err)
	}
}

// autodevFakeLLM satisfies provider.LLMProvider for registry construction.
type autodevFakeLLM struct{}

func (autodevFakeLLM) Generate(ctx context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "ok"}}, nil
}

func TestAutodevToolCallsNeedNoHumanApproval(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "test-key")
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	runner := &AgentRunner{
		workDir:        workDir,
		store:          store,
		manager:        manager,
		llmProvider:    autodevFakeLLM{},
		currentSession: sess,
	}
	runner.SetUserAsker(fakeAutodevAsker{})

	cp := checkpoint.New(checkpoint.Config{SessionDir: sess.RootDir})
	reg := runner.buildRegistry(sess, autodevFakeLLM{}, cp, func() string { return "" })

	args, _ := json.Marshal(map[string]string{"command": "echo autodev-no-approval"})
	result := reg.Execute(context.Background(), schema.ToolCall{
		ID:        "call-1",
		Name:      "bash",
		Arguments: args,
	})
	if result.IsError {
		t.Fatalf("bash tool call failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, "autodev-no-approval") {
		t.Errorf("bash output = %q, want the command to have run without any human-approval gate (TC-020)", result.Output)
	}
}
