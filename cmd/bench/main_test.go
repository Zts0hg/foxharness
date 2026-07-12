package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestBuildBenchmarkRegistryIncludesTodoTools(t *testing.T) {
	sess := &session.Session{RootDir: t.TempDir()}
	registry := buildBenchmarkRegistry(t.TempDir(), sess)
	names := map[string]bool{}
	for _, definition := range registry.GetAvailableTools() {
		names[definition.Name] = true
	}
	for _, name := range []string{"read_file", "write_file", "bash", "edit_file", "read_todo", "update_todo"} {
		if !names[name] {
			t.Fatalf("benchmark registry missing %s: %v", name, names)
		}
	}
}

func TestBenchmarkSourceHasNoLegacyPlannerPrepass(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("ReadFile(main.go) error = %v", err)
	}
	for _, forbidden := range []string{"memory.NewPlanner", ".BuildPlan("} {
		if strings.Contains(string(source), forbidden) {
			t.Fatalf("main.go still contains legacy Planner call %q", forbidden)
		}
	}
}

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
