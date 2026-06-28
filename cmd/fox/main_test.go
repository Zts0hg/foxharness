package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestParseArgsDefaultsToTUI(t *testing.T) {
	cfg, mode, err := parseArgs(nil, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if mode != launchTUI {
		t.Fatalf("mode = %v, want %v", mode, launchTUI)
	}
	if cfg.WorkDir != "." {
		t.Fatalf("WorkDir = %q, want %q", cfg.WorkDir, ".")
	}
	if cfg.Model != "" {
		t.Fatalf("Model = %q, want empty (resolved later by settings)", cfg.Model)
	}
	if cfg.LLM.ProviderID != "" {
		t.Fatalf("LLM.ProviderID = %q, want empty", cfg.LLM.ProviderID)
	}
	if cfg.LLM.Protocol != "" {
		t.Fatalf("LLM.Protocol = %q, want empty", cfg.LLM.Protocol)
	}
	if !cfg.EnablePlanMode {
		t.Fatal("EnablePlanMode = false, want true")
	}
	if cfg.MaxTurns != 0 {
		t.Fatalf("MaxTurns = %d, want 0 for unlimited", cfg.MaxTurns)
	}
}

func TestParseArgsTUIWithPositionalPrompt(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"inspect", "main.go"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if mode != launchTUI {
		t.Fatalf("mode = %v, want %v", mode, launchTUI)
	}
	if cfg.Prompt != "inspect main.go" {
		t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "inspect main.go")
	}
}

func TestParseArgsExecMode(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"exec", "inspect", "main.go"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if mode != launchPrint {
		t.Fatalf("mode = %v, want %v", mode, launchPrint)
	}
	if cfg.Prompt != "inspect main.go" {
		t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "inspect main.go")
	}
}

func TestParseArgsPrintFlag(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "short", args: []string{"-p", "inspect main.go"}},
		{name: "long", args: []string{"--print", "inspect main.go"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, mode, err := parseArgs(tt.args, io.Discard)
			if err != nil {
				t.Fatalf("parseArgs returned error: %v", err)
			}

			if mode != launchPrint {
				t.Fatalf("mode = %v, want %v", mode, launchPrint)
			}
			if cfg.Prompt != "inspect main.go" {
				t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "inspect main.go")
			}
		})
	}
}

func TestParseArgsPromptFlagKeepsDefaultTUIMode(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"-prompt", "inspect main.go"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if mode != launchTUI {
		t.Fatalf("mode = %v, want %v", mode, launchTUI)
	}
	if cfg.Prompt != "inspect main.go" {
		t.Fatalf("Prompt = %q, want %q", cfg.Prompt, "inspect main.go")
	}
}

func TestParseArgsAliases(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"-C", "/tmp/project", "-c", "-r", "session-1", "-model", "test-model", "-llm-provider", "anthropic-main", "-protocol", "claude", "-base-url", "https://api.example.test", "-auth", "api-key", "-api-key-env", "ANTHROPIC_KEY", "-api-key", "direct-key", "-max-turns", "3"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}

	if mode != launchTUI {
		t.Fatalf("mode = %v, want %v", mode, launchTUI)
	}
	if cfg.WorkDir != "/tmp/project" {
		t.Fatalf("WorkDir = %q, want %q", cfg.WorkDir, "/tmp/project")
	}
	if !cfg.ContinueSession {
		t.Fatal("ContinueSession = false, want true")
	}
	if cfg.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want %q", cfg.SessionID, "session-1")
	}
	if cfg.Model != "test-model" {
		t.Fatalf("Model = %q, want %q", cfg.Model, "test-model")
	}
	if cfg.LLM != (llmconfig.CLIOverrides{
		ProviderID: "anthropic-main",
		Protocol:   "claude",
		BaseURL:    "https://api.example.test",
		Model:      "test-model",
		Auth:       "api-key",
		APIKeyEnv:  "ANTHROPIC_KEY",
		APIKey:     "direct-key",
	}) {
		t.Fatalf("LLM = %+v, want parsed LLM overrides", cfg.LLM)
	}
	if cfg.MaxTurns != 3 {
		t.Fatalf("MaxTurns = %d, want 3", cfg.MaxTurns)
	}
}

func TestParseArgsTreatsOldProviderAsUnknownFlag(t *testing.T) {
	tests := [][]string{
		{"-provider", "openai"},
		{"--provider", "claude"},
		{"exec", "-provider=claude", "task"},
	}
	for _, args := range tests {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_, _, err := parseArgs(args, io.Discard)
			if err == nil {
				t.Fatal("parseArgs returned nil error, want unknown flag error")
			}
			if !strings.Contains(err.Error(), "flag provided but not defined") || strings.Contains(err.Error(), "-llm-provider") {
				t.Fatalf("error = %q, want generic unknown flag error", err.Error())
			}
		})
	}
}

func TestParseArgsAllowsOldProviderTextAfterPositionalPrompt(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"exec", "explain", "-provider", "usage"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if mode != launchPrint {
		t.Fatalf("mode = %v, want launchPrint", mode)
	}
	if cfg.Prompt != "explain -provider usage" {
		t.Fatalf("Prompt = %q, want positional prompt text", cfg.Prompt)
	}
}

func TestParseArgsAllowsOldProviderTextAfterFlagTerminator(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"exec", "--", "-provider", "is mentioned in docs"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if mode != launchPrint {
		t.Fatalf("mode = %v, want launchPrint", mode)
	}
	if cfg.Prompt != "-provider is mentioned in docs" {
		t.Fatalf("Prompt = %q, want prompt text after --", cfg.Prompt)
	}
}

func TestParseArgsRejectsInteractivePrintConflict(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "exec tui", args: []string{"exec", "-tui", "inspect"}},
		{name: "print interactive", args: []string{"-p", "-interactive", "inspect"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseArgs(tt.args, io.Discard)
			if err == nil {
				t.Fatal("parseArgs returned nil error, want conflict error")
			}
			if !strings.Contains(err.Error(), "-tui/-interactive") {
				t.Fatalf("error = %q, want interactive conflict", err.Error())
			}
		})
	}
}

func TestParseArgsRejectsTwoPromptSources(t *testing.T) {
	_, _, err := parseArgs([]string{"-prompt", "from flag", "from", "position"}, io.Discard)
	if err == nil {
		t.Fatal("parseArgs returned nil error, want prompt source conflict")
	}
	if !strings.Contains(err.Error(), "不能同时使用 -prompt") {
		t.Fatalf("error = %q, want prompt source conflict", err.Error())
	}
}

func TestParseArgsAutodevMode(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"autodev"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if mode != launchAutodev {
		t.Fatalf("mode = %v, want %v", mode, launchAutodev)
	}
	if cfg.Prompt != "" {
		t.Fatalf("Prompt = %q, want empty (no backlog override)", cfg.Prompt)
	}
}

func TestParseArgsAutodevWithBacklogPathAndWorkdir(t *testing.T) {
	cfg, mode, err := parseArgs([]string{"autodev", "-C", "/tmp/project", "-model", "test-model", "WORK.md"}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs returned error: %v", err)
	}
	if mode != launchAutodev {
		t.Fatalf("mode = %v, want %v", mode, launchAutodev)
	}
	if cfg.WorkDir != "/tmp/project" {
		t.Errorf("WorkDir = %q, want /tmp/project", cfg.WorkDir)
	}
	if cfg.Model != "test-model" {
		t.Errorf("Model = %q, want test-model", cfg.Model)
	}
	if cfg.Prompt != "WORK.md" {
		t.Errorf("Prompt = %q, want the backlog path positional", cfg.Prompt)
	}
}

func TestParseArgsAutodevRejectsInteractive(t *testing.T) {
	_, _, err := parseArgs([]string{"autodev", "-tui"}, io.Discard)
	if err == nil {
		t.Fatal("parseArgs returned nil error, want conflict for autodev + -tui")
	}
}

func TestResolveLLMConfigUsesSettingsDefaultProvider(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, map[string]any{
		"llm": map[string]any{
			"default_provider": "local",
			"providers": map[string]any{
				"local": map[string]any{
					"protocol": "openai",
					"base_url": "http://127.0.0.1:11434/v1",
					"model":    "local-model",
					"auth":     "none",
				},
			},
		},
	})

	got, err := resolveLLMConfig(home, llmconfig.CLIOverrides{}, mapEnv{}.Lookup)
	if err != nil {
		t.Fatalf("resolveLLMConfig() error = %v", err)
	}
	if got.ProviderID != "local" || got.Protocol != "openai" || got.Model != "local-model" {
		t.Fatalf("resolved LLM = %+v, want settings default provider", got)
	}
}

func TestResolveLLMConfigAppliesCLIAndEnvPriority(t *testing.T) {
	home := t.TempDir()
	writeSettings(t, home, map[string]any{
		"llm": map[string]any{
			"default_provider": "primary",
			"providers": map[string]any{
				"primary": map[string]any{
					"protocol":    "openai",
					"base_url":    "https://settings.test/v1",
					"model":       "settings-model",
					"api_key_env": "SETTINGS_KEY",
				},
			},
		},
	})

	got, err := resolveLLMConfig(home, llmconfig.CLIOverrides{Model: "cli-model"}, mapEnv{
		"FOXHARNESS_LLM_BASE_URL":    "https://env.test/v1",
		"FOXHARNESS_LLM_MODEL":       "env-model",
		"FOXHARNESS_LLM_API_KEY_ENV": "ENV_KEY",
		"ENV_KEY":                    "env-secret",
	}.Lookup)
	if err != nil {
		t.Fatalf("resolveLLMConfig() error = %v", err)
	}
	if got.BaseURL != "https://env.test/v1" {
		t.Fatalf("BaseURL = %q, want env override", got.BaseURL)
	}
	if got.Model != "cli-model" {
		t.Fatalf("Model = %q, want CLI override", got.Model)
	}
	if got.APIKey != "env-secret" {
		t.Fatal("API key was not resolved from env override")
	}
}

func TestResolveLLMConfigDoesNotUseLegacyFallbacks(t *testing.T) {
	home := t.TempDir()
	_, err := resolveLLMConfig(home, llmconfig.CLIOverrides{}, mapEnv{
		"FOX_MODEL":     "legacy-model",
		"ZHIPU_API_KEY": "legacy-key",
	}.Lookup)
	if err == nil {
		t.Fatal("resolveLLMConfig() error = nil, want missing config")
	}
	if strings.Contains(err.Error(), "ZHIPU_API_KEY") || strings.Contains(err.Error(), "glm-4.5-air") {
		t.Fatalf("error = %q, want no legacy fallback guidance", err.Error())
	}
}

func TestExitCodeForError(t *testing.T) {
	if got := exitCodeForError(nil); got != 0 {
		t.Errorf("exitCodeForError(nil) = %d, want 0 (backlog drained)", got)
	}
	if got := exitCodeForError(&autodev.PreconditionError{Reason: "gh missing"}); got != 2 {
		t.Errorf("exitCodeForError(precondition) = %d, want 2", got)
	}
	if got := exitCodeForError(fmt.Errorf("wrapped: %w", &autodev.PreconditionError{Reason: "not a repo"})); got != 2 {
		t.Errorf("exitCodeForError(wrapped precondition) = %d, want 2", got)
	}
	if got := exitCodeForError(errors.New("boom")); got != 1 {
		t.Errorf("exitCodeForError(unexpected) = %d, want 1", got)
	}
}

type mapEnv map[string]string

func (m mapEnv) Lookup(name string) string {
	return m[name]
}

func writeSettings(t *testing.T, home string, value map[string]any) {
	t.Helper()
	dir := filepath.Join(home, ".foxharness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadPromptUsesInlinePrompt(t *testing.T) {
	prompt, err := readPrompt(" inspect main.go ")
	if err != nil {
		t.Fatalf("readPrompt returned error: %v", err)
	}
	if prompt != "inspect main.go" {
		t.Fatalf("prompt = %q, want %q", prompt, "inspect main.go")
	}
}

func TestReadPromptReadsStdinForDash(t *testing.T) {
	previousStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = previousStdin
		_ = r.Close()
	}()

	if _, err := w.WriteString("inspect main.go\n"); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}

	prompt, err := readPrompt("-")
	if err != nil {
		t.Fatalf("readPrompt returned error: %v", err)
	}
	if prompt != "inspect main.go" {
		t.Fatalf("prompt = %q, want %q", prompt, "inspect main.go")
	}
}
