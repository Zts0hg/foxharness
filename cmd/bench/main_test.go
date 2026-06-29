package main

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestResolveBenchmarkLLMConfigUsesScopedEnvironment(t *testing.T) {
	got, err := resolveBenchmarkLLMConfig(t.TempDir(), mapEnv{
		"FOXHARNESS_LLM_PROTOCOL": "openai",
		"FOXHARNESS_LLM_BASE_URL": "http://127.0.0.1:11434/v1",
		"FOXHARNESS_LLM_MODEL":    "local-model",
		"FOXHARNESS_LLM_AUTH":     llmconfig.AuthNone,
		"ZHIPU_API_KEY":           "legacy-key",
	}.Lookup)
	if err != nil {
		t.Fatalf("resolveBenchmarkLLMConfig() error = %v", err)
	}
	if got.Protocol != llmconfig.ProtocolOpenAI || got.BaseURL != "http://127.0.0.1:11434/v1" || got.Model != "local-model" {
		t.Fatalf("resolved config = %+v, want scoped env values", got)
	}
	if got.APIKey != "" {
		t.Fatal("APIKey was resolved despite auth none")
	}
}

func TestResolveBenchmarkLLMConfigMissingConfigDoesNotMentionLegacyDefaults(t *testing.T) {
	_, err := resolveBenchmarkLLMConfig(t.TempDir(), mapEnv{
		"FOX_MODEL":     "legacy-model",
		"ZHIPU_API_KEY": "legacy-key",
	}.Lookup)
	if err == nil {
		t.Fatal("resolveBenchmarkLLMConfig() error = nil, want missing config")
	}
	if strings.Contains(err.Error(), "ZHIPU_API_KEY") || strings.Contains(err.Error(), "glm-4.5-air") {
		t.Fatalf("error = %q, want no legacy fallback guidance", err.Error())
	}
}

type mapEnv map[string]string

func (m mapEnv) Lookup(name string) string {
	return m[name]
}
