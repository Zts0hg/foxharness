package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestDeduperReclaimsExpiredMessageIDs(t *testing.T) {
	now := time.Unix(1000, 0)
	deduper := NewDeduperWithTTL(time.Minute)
	deduper.now = func() time.Time {
		return now
	}

	if !deduper.Mark("message-1") {
		t.Fatalf("first message should be accepted")
	}
	if deduper.Mark("message-1") {
		t.Fatalf("recent duplicate should be rejected")
	}
	now = now.Add(2 * time.Minute)
	if !deduper.Mark("message-1") {
		t.Fatalf("expired message should be accepted again after cleanup")
	}
	if got := deduper.Len(); got != 1 {
		t.Fatalf("dedupe entries = %d, want 1 after cleanup", got)
	}
}

func TestDeduperCleanupKeepsRecentMessageIDs(t *testing.T) {
	now := time.Unix(1000, 0)
	deduper := NewDeduperWithTTL(time.Minute)
	deduper.now = func() time.Time {
		return now
	}

	if !deduper.Mark("message-1") {
		t.Fatalf("first message should be accepted")
	}
	now = now.Add(30 * time.Second)
	deduper.Cleanup()
	if got := deduper.Len(); got != 1 {
		t.Fatalf("dedupe entries = %d, want recent entry preserved", got)
	}
	if deduper.Mark("message-1") {
		t.Fatalf("recent duplicate should remain rejected after cleanup")
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
