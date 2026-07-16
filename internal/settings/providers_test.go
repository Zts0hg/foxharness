package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

func TestSetProviderUpsertsAndCreatesMap(t *testing.T) {
	s := &Settings{}
	profile := llmconfig.Profile{Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://example.test/v1", Model: "m", APIKeyEnv: "K"}
	if err := SetProvider(s, "primary", profile); err != nil {
		t.Fatalf("SetProvider() error = %v", err)
	}
	if got := s.LLM.Providers["primary"]; got != profile {
		t.Fatalf("Providers[primary] = %+v, want %+v", got, profile)
	}
	// Upsert overwrites an existing profile with the same id.
	updated := profile
	updated.Model = "m2"
	if err := SetProvider(s, "primary", updated); err != nil {
		t.Fatalf("SetProvider() overwrite error = %v", err)
	}
	if got := s.LLM.Providers["primary"].Model; got != "m2" {
		t.Fatalf("Model = %q, want overwritten m2", got)
	}
}

func TestSetProviderRejectsInvalidInput(t *testing.T) {
	if err := SetProvider(nil, "p", llmconfig.Profile{}); err == nil {
		t.Fatal("SetProvider(nil) error = nil, want error")
	}
	if err := SetProvider(&Settings{}, "  ", llmconfig.Profile{}); err == nil {
		t.Fatal("SetProvider(empty id) error = nil, want error")
	}
}

func TestSetProviderPersistPreservesUnknownFieldsAnd0600(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".foxharness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := SetProvider(loaded, "primary", llmconfig.Profile{
		Protocol: llmconfig.ProtocolOpenAI, BaseURL: "https://example.test/v1", Model: "m", APIKeyEnv: "K",
	}); err != nil {
		t.Fatalf("SetProvider() error = %v", err)
	}
	if err := Save(home, loaded); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed["theme"] != "dark" {
		t.Fatalf("theme = %v, want preserved unknown field", parsed["theme"])
	}
	providers := parsed["llm"].(map[string]any)["providers"].(map[string]any)
	if providers["primary"] == nil {
		t.Fatal("primary provider not persisted")
	}
	info, err := os.Stat(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %04o, want 0600", info.Mode().Perm())
	}
}

func TestSetDefaultProviderSetsAndRejectsUnknown(t *testing.T) {
	s := &Settings{LLM: llmconfig.Settings{Providers: map[string]llmconfig.Profile{"p": {Model: "m"}}}}
	if err := SetDefaultProvider(s, "p"); err != nil {
		t.Fatalf("SetDefaultProvider() error = %v", err)
	}
	if s.LLM.DefaultProvider != "p" {
		t.Fatalf("DefaultProvider = %q, want p", s.LLM.DefaultProvider)
	}
	if err := SetDefaultProvider(s, "missing"); err == nil {
		t.Fatal("SetDefaultProvider(unknown) error = nil, want error")
	}
	if err := SetDefaultProvider(nil, "p"); err == nil {
		t.Fatal("SetDefaultProvider(nil) error = nil, want error")
	}
}

func TestLoadAndSaveLLMEffort(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".foxharness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{
	  "llm": {
	    "future": "preserve",
	    "effort": {
	      "openai": "minimal",
	      "claude": "max"
	    }
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.LLM.Effort[llmconfig.ProtocolOpenAI] != "minimal" {
		t.Fatalf("openai effort = %q, want minimal", loaded.LLM.Effort[llmconfig.ProtocolOpenAI])
	}
	if loaded.LLM.Effort[llmconfig.ProtocolClaude] != "max" {
		t.Fatalf("claude effort = %q, want max", loaded.LLM.Effort[llmconfig.ProtocolClaude])
	}

	if err := SetEffort(loaded, llmconfig.ProtocolOpenAI, "high"); err != nil {
		t.Fatalf("SetEffort(openai high) error = %v", err)
	}
	if err := SetEffort(loaded, llmconfig.ProtocolClaude, "auto"); err != nil {
		t.Fatalf("SetEffort(claude auto) error = %v", err)
	}
	if err := Save(home, loaded); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	llm := parsed["llm"].(map[string]any)
	if llm["future"] != "preserve" {
		t.Fatalf("llm.future = %v, want preserved", llm["future"])
	}
	effort := llm["effort"].(map[string]any)
	if effort["openai"] != "high" {
		t.Fatalf("openai effort = %v, want high", effort["openai"])
	}
	if _, ok := effort["claude"]; ok {
		t.Fatal("claude effort still persisted after auto clear")
	}
}

func TestSetEffortAutoClearsLastPersistedEffort(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".foxharness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{
	  "llm": {
	    "effort": {
	      "openai": "minimal"
	    }
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if err := SetEffort(loaded, llmconfig.ProtocolOpenAI, "auto"); err != nil {
		t.Fatalf("SetEffort(auto) error = %v", err)
	}
	if err := Save(home, loaded); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	reloaded, err := Load(home)
	if err != nil {
		t.Fatalf("reload error = %v", err)
	}
	if got := reloaded.LLM.Effort[llmconfig.ProtocolOpenAI]; got != "" {
		t.Fatalf("openai effort = %q, want cleared", got)
	}
}

func TestSetEffortRejectsInvalidInput(t *testing.T) {
	if err := SetEffort(nil, llmconfig.ProtocolOpenAI, "low"); err == nil {
		t.Fatal("SetEffort(nil) error = nil, want error")
	}
	if err := SetEffort(&Settings{}, llmconfig.ProtocolClaude, "minimal"); err == nil {
		t.Fatal("SetEffort(invalid value) error = nil, want error")
	}
}
