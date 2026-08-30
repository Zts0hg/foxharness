package main

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tui"
)

/* TestTUIRunKeepsBaselineBasePromptRules verifies that the TUI model-visible
 * system prompt keeps the baseline base rules even though the composition
 * knows the run's tool capabilities. */
func TestTUIRunKeepsBaselineBasePromptRules(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := &systemPromptTUIProvider{}
	config := foxConfig{
		WorkDir: t.TempDir(), Model: "tui-model", MaxTurns: 4,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := startup.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := startup.Application.Run(context.Background(), app.RunCommand{Prompt: "hello"}, nil); err != nil {
		t.Fatal(err)
	}
	prompt := model.systemPrompt()
	for _, want := range []string{
		"Prefer reading files before editing them.",
		"After changing code, verify with the smallest relevant test command.",
		"Treat @path tokens in user messages as project-relative file references; read referenced files before making claims or edits about them.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("TUI system prompt is missing the baseline rule %q:\n%s", want, prompt)
		}
	}
}

/* TestTUIRestrictedRunKeepsBaselineBasePromptRules verifies that an
 * allowed-tools restriction filters the run's tool surface without replacing
 * the baseline base system prompt, matching the baseline main-run behavior. */
func TestTUIRestrictedRunKeepsBaselineBasePromptRules(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := &systemPromptTUIProvider{}
	config := foxConfig{
		WorkDir: t.TempDir(), Model: "tui-model", MaxTurns: 4,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := startup.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()
	if _, err := startup.Application.Run(context.Background(), app.RunCommand{
		Prompt: "hello", AllowedTools: []string{"read_file"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	prompt := model.systemPrompt()
	for _, want := range []string{
		"Prefer reading files before editing them.",
		"After changing code, verify with the smallest relevant test command.",
		"Treat @path tokens in user messages as project-relative file references; read referenced files before making claims or edits about them.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("restricted TUI system prompt is missing the baseline rule %q:\n%s", want, prompt)
		}
	}
}

/* systemPromptTUIProvider records the system prompt the model receives. */
type systemPromptTUIProvider struct {
	mu     sync.Mutex
	prompt string
}

func (p *systemPromptTUIProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	return p.GenerateWithOptions(ctx, messages, definitions, provider.GenerateOptions{})
}

func (p *systemPromptTUIProvider) GenerateWithOptions(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition, _ provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	for _, message := range messages {
		if message.Role == schema.RoleSystem {
			p.prompt = message.Content
		}
	}
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *systemPromptTUIProvider) systemPrompt() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.prompt
}
