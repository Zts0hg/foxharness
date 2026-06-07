package keeprun

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const (
	wantClarifyDefault   = "Make decisions that prioritize correctness, simplicity, and alignment with project conventions."
	wantReviewFixDefault = "Fix all issues, warnings, and suggestions. Prioritize correctness and code quality. Follow project constitution and TDD principles."
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if !cfg.RemoteEnabled {
		t.Errorf("RemoteEnabled = false, want true")
	}
	if cfg.ReviewMode != "subagent" {
		t.Errorf("ReviewMode = %q, want %q", cfg.ReviewMode, "subagent")
	}
	if cfg.ClarifyPrompt != wantClarifyDefault {
		t.Errorf("ClarifyPrompt = %q, want %q", cfg.ClarifyPrompt, wantClarifyDefault)
	}
	if cfg.ReviewFixPrompt != wantReviewFixDefault {
		t.Errorf("ReviewFixPrompt = %q, want %q", cfg.ReviewFixPrompt, wantReviewFixDefault)
	}
	if cfg.RetryPolicy.Backoff != "exponential" {
		t.Errorf("RetryPolicy.Backoff = %q, want %q", cfg.RetryPolicy.Backoff, "exponential")
	}
}

func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "keep-run.config.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("valid_complete_config", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, `{
			"remote_enabled": false,
			"review_mode": "direct",
			"clarify_prompt": "custom clarify",
			"review_fix_prompt": "custom fix",
			"retry_policy": {"backoff": "linear"}
		}`)
		cfg, err := LoadConfig(dir)
		if err != nil {
			t.Fatalf("LoadConfig error: %v", err)
		}
		want := Config{
			RemoteEnabled:   false,
			ReviewMode:      "direct",
			ClarifyPrompt:   "custom clarify",
			ReviewFixPrompt: "custom fix",
			RetryPolicy:     RetryPolicy{Backoff: "linear"},
		}
		if !reflect.DeepEqual(cfg, want) {
			t.Errorf("LoadConfig = %+v, want %+v", cfg, want)
		}
	})

	t.Run("missing_file_returns_defaults", func(t *testing.T) {
		dir := t.TempDir()
		cfg, err := LoadConfig(dir)
		if err != nil {
			t.Fatalf("LoadConfig error: %v", err)
		}
		if !reflect.DeepEqual(cfg, DefaultConfig()) {
			t.Errorf("LoadConfig (missing file) = %+v, want defaults %+v", cfg, DefaultConfig())
		}
	})

	t.Run("partial_config_fills_defaults", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, `{"review_mode": "direct"}`)
		cfg, err := LoadConfig(dir)
		if err != nil {
			t.Fatalf("LoadConfig error: %v", err)
		}
		if cfg.ReviewMode != "direct" {
			t.Errorf("ReviewMode = %q, want direct (overridden)", cfg.ReviewMode)
		}
		if !cfg.RemoteEnabled {
			t.Errorf("RemoteEnabled = false, want true (default)")
		}
		if cfg.RetryPolicy.Backoff != "exponential" {
			t.Errorf("RetryPolicy.Backoff = %q, want exponential (default)", cfg.RetryPolicy.Backoff)
		}
		if cfg.ClarifyPrompt != wantClarifyDefault {
			t.Errorf("ClarifyPrompt = %q, want default", cfg.ClarifyPrompt)
		}
	})

	t.Run("empty_object_all_defaults", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, `{}`)
		cfg, err := LoadConfig(dir)
		if err != nil {
			t.Fatalf("LoadConfig error: %v", err)
		}
		if !reflect.DeepEqual(cfg, DefaultConfig()) {
			t.Errorf("LoadConfig (empty object) = %+v, want defaults %+v", cfg, DefaultConfig())
		}
	})

	t.Run("invalid_json_returns_error", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, `{not valid json`)
		if _, err := LoadConfig(dir); err == nil {
			t.Fatal("LoadConfig with invalid JSON: expected error, got nil")
		}
	})

	t.Run("explicit_remote_disabled_respected", func(t *testing.T) {
		dir := t.TempDir()
		writeConfig(t, dir, `{"remote_enabled": false}`)
		cfg, err := LoadConfig(dir)
		if err != nil {
			t.Fatalf("LoadConfig error: %v", err)
		}
		if cfg.RemoteEnabled {
			t.Error("RemoteEnabled = true, want false (explicitly disabled)")
		}
	})
}
