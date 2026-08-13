package app

import (
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestUIAUT006TUILaunchPreservesTypedAutodevConfig(t *testing.T) {
	original := CLIConfig{
		WorkDir:        "/repo",
		Prompt:         "interactive initial prompt",
		Model:          "model-x",
		LLM:            llmconfig.CLIOverrides{ProviderID: "fixture", Protocol: "claude", BaseURL: "http://fixture", Auth: "none"},
		ResolvedLLM:    llmconfig.ResolvedConfig{ProviderID: "fixture", Protocol: "claude", BaseURL: "http://fixture", Auth: "none", Model: "model-x"},
		EffortOverride: "high",
		EnableThinking: true,
		MaxTurns:       17,
		SessionID:      "session-1",
		NewSession:     true,
		Interactive:    true,
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
