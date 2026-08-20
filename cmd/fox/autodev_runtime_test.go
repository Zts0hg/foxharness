package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestUIAUT006TUILaunchPreservesTypedAutodevConfig(t *testing.T) {
	original := app.CLIConfig{
		WorkDir: "/repo", Prompt: "interactive initial prompt", Model: "model-x",
		LLM:            llmconfig.CLIOverrides{ProviderID: "fixture", Protocol: "claude", BaseURL: "http://fixture", Auth: "none"},
		ResolvedLLM:    llmconfig.ResolvedConfig{ProviderID: "fixture", Protocol: "claude", BaseURL: "http://fixture", Auth: "none", Model: "model-x"},
		EffortOverride: "high", EnableThinking: true, MaxTurns: 17,
		SessionID: "session-1", NewSession: true, Interactive: true,
	}
	want := original
	want.Prompt = "WORK.md"
	got := autodevConfigForTUILaunch(original, "WORK.md")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TUI autodev launch config = %#v, want %#v", got, want)
	}
	if original.Prompt != "interactive initial prompt" {
		t.Fatalf("source config was mutated: %#v", original)
	}
}

func TestM18AutodevTargetFactoryPreservesRuntimeProfileAndEngineerQuestionPort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte("AUTODEV_TARGET_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(workDir, ".claude", "commands", "codexspec", "generate-spec.md")
	if err := os.MkdirAll(filepath.Dir(commandPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(commandPath, []byte("Generate target spec for $ARGUMENTS"), 0o644); err != nil {
		t.Fatal(err)
	}
	scripted := &targetAutodevProvider{}
	var configs []llmconfig.ResolvedConfig
	factory := &autodevRuntimeCoreFactory{
		llmConfig: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "base-model"},
		maxTurns:  4,
		newProvider: func(config llmconfig.ResolvedConfig) (provider.LLMProvider, error) {
			configs = append(configs, config)
			return scripted, nil
		},
	}
	core, err := factory.New(context.Background(), workDir, "item-model")
	if err != nil {
		t.Fatal(err)
	}
	asker := &targetAutodevAsker{}
	core.SetUserAsker(asker)
	stagePrompt, err := core.StagePrompt(context.Background(), "codexspec:generate-spec", "feature/requirements.md")
	if err != nil || stagePrompt != "Generate target spec for feature/requirements.md" {
		t.Fatalf("StagePrompt = %q, %v", stagePrompt, err)
	}
	outcome := core.Run(context.Background(), autodev.CoreAttempt{
		AttemptID: "attempt-1", CorrelationID: "attempt-1", Ordinal: 1, Prompt: "implement item",
	}, nil)
	if outcome.Status != autodev.CoreOutcomeSucceeded || outcome.PartialMessage != "question answered" {
		t.Fatalf("outcome = %#v, cause = %v", outcome, outcome.Cause)
	}
	if len(asker.questions) != 1 || asker.questions[0].Prompt != "Choose implementation" {
		t.Fatalf("Engineer questions = %#v", asker.questions)
	}
	if err := core.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(configs) != 1 || configs[0].Model != "item-model" {
		t.Fatalf("provider configs = %#v", configs)
	}

	observations := scripted.snapshot()
	if len(observations) < 2 {
		t.Fatalf("provider observations = %d, want core tool continuation", len(observations))
	}
	first := observations[0]
	if !strings.Contains(first.messages[0].Content, "AUTODEV_TARGET_INSTRUCTION") {
		t.Fatalf("system prompt omitted project instruction:\n%s", first.messages[0].Content)
	}
	var names []string
	for _, definition := range first.definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	if got, want := strings.Join(names, ","), "AskUserQuestion,ask_user_question,bash,delegate_task,edit_file,read_file,read_todo,skill,update_todo,write_file"; got != want {
		t.Fatalf("tool surface = %q, want %q", got, want)
	}

	stored, err := session.NewFileStore(workDir).Open(session.ID(outcome.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Source != session.SOURCECLI || stored.WorkDir != workDir {
		t.Fatalf("stored session = %#v", stored)
	}
	records, err := session.NewMessageLog(stored).LoadRecords()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 4 || records[0].Message.Content != "implement item" || records[3].Message.Content != "question answered" {
		t.Fatalf("persisted records = %#v", records)
	}
	for _, path := range []string{
		stored.TranscriptPath(),
		filepath.Join(stored.RootDir, "runs", outcome.RunID, "metrics.jsonl"),
		filepath.Join(stored.RootDir, "runs", outcome.RunID, "trace.jsonl"),
		filepath.Join(stored.RootDir, "checkpoints.jsonl"),
		filepath.Join(stored.RootDir, "state_history.jsonl"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected Autodev artifact %q: %v", path, err)
		}
	}
}

func TestM18AutodevTargetFactoryCreatesFreshSessionsAndSwitchesFutureModel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	var mu sync.Mutex
	var models []string
	factory := &autodevRuntimeCoreFactory{
		llmConfig: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "base"},
		maxTurns:  1,
		newProvider: func(config llmconfig.ResolvedConfig) (provider.LLMProvider, error) {
			mu.Lock()
			models = append(models, config.Model)
			mu.Unlock()
			return &targetAutodevFinalProvider{}, nil
		},
	}
	first, err := factory.New(context.Background(), workDir, "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := factory.New(context.Background(), workDir, "second")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SetModel("switched"); err != nil {
		t.Fatal(err)
	}
	firstOutcome := first.Run(context.Background(), autodev.CoreAttempt{AttemptID: "a", CorrelationID: "a", Ordinal: 1, Prompt: "first"}, nil)
	if err := first.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
	continuedOutcome := first.Run(context.Background(), autodev.CoreAttempt{AttemptID: "a2", CorrelationID: "a2", Ordinal: 2, Prompt: "continue"}, nil)
	secondOutcome := second.Run(context.Background(), autodev.CoreAttempt{AttemptID: "b", CorrelationID: "b", Ordinal: 1, Prompt: "second"}, nil)
	if firstOutcome.SessionID == "" || secondOutcome.SessionID == "" || firstOutcome.SessionID == secondOutcome.SessionID {
		t.Fatalf("fresh session IDs = %q/%q", firstOutcome.SessionID, secondOutcome.SessionID)
	}
	if continuedOutcome.SessionID != firstOutcome.SessionID || continuedOutcome.RunID == "" || continuedOutcome.RunID == firstOutcome.RunID {
		t.Fatalf("same-item continuation = %#v after %#v", continuedOutcome, firstOutcome)
	}
	if err := first.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotModels := append([]string(nil), models...)
	mu.Unlock()
	if got, want := strings.Join(gotModels, ","), "first,second,switched"; got != want {
		t.Fatalf("provider model sequence = %q, want %q", got, want)
	}
}

func TestM18BuildRuntimeAutodevDepsUsesTargetFactoryAndSharedModel(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".foxharness"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".foxharness", "autodev.yml"), []byte("model: shared-model\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, err := buildRuntimeAutodevDeps(context.Background(), app.CLIConfig{
		WorkDir: repoRoot, Model: "cli-model",
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "http://127.0.0.1:1", Model: "cli-model", Auth: llmconfig.AuthNone},
	}, autodev.NewTerminalReporter(os.Stderr))
	if err != nil {
		t.Fatal(err)
	}
	if deps.Config.Model != "shared-model" {
		t.Fatalf("resolved model = %q", deps.Config.Model)
	}
	if _, ok := deps.CoreFactory.(*autodevRuntimeCoreFactory); !ok {
		t.Fatalf("CoreFactory = %T, want runtime-backed factory", deps.CoreFactory)
	}
	engineer, ok := deps.Engineer.(*autodev.ProviderEngineerAgent)
	if !ok || engineer.Model() != "shared-model" {
		t.Fatalf("Engineer = %T/%v", deps.Engineer, ok)
	}
}

type targetAutodevObservation struct {
	messages    []schema.Message
	definitions []schema.ToolDefinition
}

type targetAutodevProvider struct {
	mu           sync.Mutex
	calls        int
	observations []targetAutodevObservation
}

func (p *targetAutodevProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *targetAutodevProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, definitions []schema.ToolDefinition, _ provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.observations = append(p.observations, targetAutodevObservation{
		messages: append([]schema.Message(nil), messages...), definitions: append([]schema.ToolDefinition(nil), definitions...),
	})
	p.mu.Unlock()
	if call == 1 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{
			ID: "ask-1", Name: "AskUserQuestion", Arguments: []byte(`{"questions":[{"question":"Choose implementation","header":"Choice","options":[{"label":"A","description":"first"},{"label":"B","description":"second"}]}]}`),
		}}}}, nil
	}
	if call == 2 {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "question answered"}}, nil
	}
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
}

func (p *targetAutodevProvider) snapshot() []targetAutodevObservation {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]targetAutodevObservation(nil), p.observations...)
}

type targetAutodevAsker struct{ questions []autodev.Question }

func (a *targetAutodevAsker) Ask(_ context.Context, questions []autodev.Question) ([]autodev.Answer, error) {
	a.questions = append([]autodev.Question(nil), questions...)
	return []autodev.Answer{{QuestionText: questions[0].Prompt, Value: "A"}}, nil
}

type targetAutodevFinalProvider struct{}

func (*targetAutodevFinalProvider) Generate(context.Context, []schema.Message, []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}
