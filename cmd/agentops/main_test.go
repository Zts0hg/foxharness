package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
)

func TestConfiguredLLMProviderUsesSettingsProfileWithoutLegacyZhipu(t *testing.T) {
	home := t.TempDir()
	writeLLMSettings(t, home)

	got, err := newConfiguredLLMProvider(home, mapEnv{"ZHIPU_API_KEY": "legacy-key"}.Lookup)
	if err != nil {
		t.Fatalf("newConfiguredLLMProvider() error = %v", err)
	}
	if _, ok := got.(*provider.OpenAIProvider); !ok {
		t.Fatalf("provider = %T, want *provider.OpenAIProvider", got)
	}
}

func TestConfiguredLLMProviderMissingConfigDoesNotMentionLegacyDefaults(t *testing.T) {
	_, err := newConfiguredLLMProvider(t.TempDir(), mapEnv{
		"FOX_MODEL":     "legacy-model",
		"ZHIPU_API_KEY": "legacy-key",
	}.Lookup)
	if err == nil {
		t.Fatal("newConfiguredLLMProvider() error = nil, want missing config")
	}
	if strings.Contains(err.Error(), "ZHIPU_API_KEY") || strings.Contains(err.Error(), "glm-4.5-air") {
		t.Fatalf("error = %q, want no legacy fallback guidance", err.Error())
	}
}

type mapEnv map[string]string

func (m mapEnv) Lookup(name string) string {
	return m[name]
}

func writeLLMSettings(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".foxharness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{
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
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
