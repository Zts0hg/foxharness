package main

import (
	"io"
	"os"
	"strings"
	"testing"
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
	if cfg.Provider != "openai" {
		t.Fatalf("Provider = %q, want openai", cfg.Provider)
	}
	if !cfg.EnablePlanMode {
		t.Fatal("EnablePlanMode = false, want true")
	}
	if cfg.MaxTurns != 20 {
		t.Fatalf("MaxTurns = %d, want 20", cfg.MaxTurns)
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
	cfg, mode, err := parseArgs([]string{"-C", "/tmp/project", "-c", "-r", "session-1", "-model", "test-model", "-provider", "claude", "-max-turns", "3"}, io.Discard)
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
	if cfg.Provider != "claude" {
		t.Fatalf("Provider = %q, want claude", cfg.Provider)
	}
	if cfg.MaxTurns != 3 {
		t.Fatalf("MaxTurns = %d, want 3", cfg.MaxTurns)
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
