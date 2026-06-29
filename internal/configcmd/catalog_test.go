package configcmd

import (
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestCatalogHasTwelvePresets(t *testing.T) {
	if len(Catalog) != 12 {
		t.Fatalf("len(Catalog) = %d, want 12", len(Catalog))
	}
}

func TestCatalogPresetFields(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Catalog {
		if p.ID == "" || p.BaseURL == "" || p.Model == "" {
			t.Errorf("preset %+v has an empty required field", p)
		}
		if p.Protocol != llmconfig.ProtocolOpenAI && p.Protocol != llmconfig.ProtocolClaude {
			t.Errorf("preset %q protocol = %q, want openai or claude", p.ID, p.Protocol)
		}
		if seen[p.ID] {
			t.Errorf("preset id %q duplicated", p.ID)
		}
		seen[p.ID] = true
		if p.Auth == llmconfig.AuthNone {
			if p.APIKeyEnv != "" {
				t.Errorf("preset %q auth=none but APIKeyEnv=%q", p.ID, p.APIKeyEnv)
			}
			continue
		}
		if p.APIKeyEnv == "" {
			t.Errorf("preset %q has empty APIKeyEnv for keyed auth", p.ID)
		}
	}
	for _, id := range []string{
		"openai", "anthropic", "xai", "mistral", "groq", "openrouter",
		"zhipu", "deepseek", "moonshot", "qwen", "minimax", "ollama",
	} {
		if !seen[id] {
			t.Errorf("catalog missing preset %q", id)
		}
	}
}

func TestCatalogSpecificProtocols(t *testing.T) {
	if p, ok := PresetByID("anthropic"); !ok || p.Protocol != llmconfig.ProtocolClaude {
		t.Errorf("anthropic = %+v ok=%v, want protocol claude", p, ok)
	}
	if p, ok := PresetByID("ollama"); !ok || p.Auth != llmconfig.AuthNone {
		t.Errorf("ollama = %+v ok=%v, want auth none", p, ok)
	}
	if _, ok := PresetByID("does-not-exist"); ok {
		t.Fatal("PresetByID returned ok for unknown id")
	}
}
