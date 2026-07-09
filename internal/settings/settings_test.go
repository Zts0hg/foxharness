package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

// ---------------------------------------------------------------------------
// Load tests (Task 1.2)
// ---------------------------------------------------------------------------

func TestLoad(t *testing.T) {
	t.Run("missing_file_returns_zero_value", func(t *testing.T) {
		home := t.TempDir()
		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.Model != "" {
			t.Fatalf("Model = %q, want empty", got.Model)
		}
	})

	t.Run("valid_file", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, filepath.Join(dir, "settings.json"), map[string]string{"model": "glm-4-plus"})

		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.Model != "glm-4-plus" {
			t.Fatalf("Model = %q, want glm-4-plus", got.Model)
		}
	})

	t.Run("malformed_json_returns_zero_value", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{bad"), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.Model != "" {
			t.Fatalf("Model = %q, want empty for malformed JSON", got.Model)
		}
	})

	t.Run("empty_model_field", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, filepath.Join(dir, "settings.json"), map[string]string{"model": ""})

		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.Model != "" {
			t.Fatalf("Model = %q, want empty", got.Model)
		}
	})

	t.Run("extra_fields_preserved_in_raw", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		raw := `{"model": "glm-4-plus", "future_field": "hello"}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.Model != "glm-4-plus" {
			t.Fatalf("Model = %q, want glm-4-plus", got.Model)
		}
		// Verify raw bytes were captured for round-trip.
		var m map[string]json.RawMessage
		if err := json.Unmarshal(got.raw, &m); err != nil {
			t.Fatalf("unmarshal raw: %v", err)
		}
		if _, ok := m["future_field"]; !ok {
			t.Fatal("future_field missing from raw bytes")
		}
	})

	t.Run("nonexistent_directory_returns_zero_value", func(t *testing.T) {
		home := filepath.Join(t.TempDir(), "nope")
		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.Model != "" {
			t.Fatalf("Model = %q, want empty", got.Model)
		}
	})

	t.Run("loads_llm_provider_settings", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		raw := `{
		  "llm": {
		    "default_provider": "primary",
		    "providers": {
		      "primary": {
		        "protocol": "openai",
		        "base_url": "https://example.test/v1",
		        "model": "test-model",
		        "auth": "api-key",
		        "api_key_env": "OPENAI_KEY"
		      }
		    }
		  }
		}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.LLM.DefaultProvider != "primary" {
			t.Fatalf("DefaultProvider = %q, want primary", got.LLM.DefaultProvider)
		}
		profile := got.LLM.Providers["primary"]
		if profile.Protocol != llmconfig.ProtocolOpenAI || profile.BaseURL != "https://example.test/v1" || profile.Model != "test-model" || profile.APIKeyEnv != "OPENAI_KEY" {
			t.Fatalf("profile = %+v, want loaded LLM provider", profile)
		}
	})

	t.Run("loads_tui_settings", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		raw := `{
		  "tui": {
		    "theme": "mono",
		    "statusline": ["model", "project", "queued"]
		  }
		}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(raw), 0644); err != nil {
			t.Fatal(err)
		}

		got, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if got.TUI.Theme != "mono" {
			t.Fatalf("TUI.Theme = %q, want mono", got.TUI.Theme)
		}
		wantStatusline := []string{"model", "project", "queued"}
		if !reflect.DeepEqual(got.TUI.Statusline, wantStatusline) {
			t.Fatalf("TUI.Statusline = %#v, want %#v", got.TUI.Statusline, wantStatusline)
		}
	})
}

// ---------------------------------------------------------------------------
// Save tests (Task 1.4)
// ---------------------------------------------------------------------------

func TestSave(t *testing.T) {
	t.Run("creates_directory_if_missing", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{}
		if err := Save(home, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		path := filepath.Join(home, ".foxharness", "settings.json")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Fatal("settings.json not created")
		}
	})

	t.Run("writes_valid_json", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{
			LLM: llmconfig.Settings{
				DefaultProvider: "primary",
				Providers: map[string]llmconfig.Profile{
					"primary": {
						Protocol: llmconfig.ProtocolOpenAI,
						BaseURL:  "https://example.test/v1",
						Model:    "test-model",
						Auth:     llmconfig.AuthNone,
					},
				},
			},
		}
		if err := Save(home, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		path := filepath.Join(home, ".foxharness", "settings.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			LLM llmconfig.Settings `json:"llm"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed.LLM.DefaultProvider != "primary" {
			t.Fatalf("DefaultProvider = %q, want primary", parsed.LLM.DefaultProvider)
		}
		if parsed.LLM.Providers["primary"].Model != "test-model" {
			t.Fatalf("provider model = %q, want test-model", parsed.LLM.Providers["primary"].Model)
		}
	})

	t.Run("file_permissions_0600", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{}
		if err := Save(home, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		path := filepath.Join(home, ".foxharness", "settings.json")
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Fatalf("permissions = %04o, want 0600", perm)
		}
	})

	t.Run("preserves_unknown_fields", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		original := `{"model": "old", "theme": "dark", "version": 2}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(original), 0644); err != nil {
			t.Fatal(err)
		}

		// Load, update the legacy top-level model field, save.
		got, _ := Load(home)
		got.Model = "glm-4-plus"
		if err := Save(home, got); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed["model"] != "old" {
			t.Fatalf("model = %v, want old legacy value preserved", parsed["model"])
		}
		if parsed["theme"] != "dark" {
			t.Fatalf("theme = %v, want dark (unknown field lost)", parsed["theme"])
		}
		if parsed["version"] != float64(2) {
			t.Fatalf("version = %v, want 2 (unknown field lost)", parsed["version"])
		}
	})

	t.Run("atomic_write_no_temp_file_left", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{}
		if err := Save(home, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		dir := filepath.Join(home, ".foxharness")
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Name() != "settings.json" {
				t.Fatalf("unexpected file in .foxharness: %s (temp file leaked)", e.Name())
			}
		}
	})

	t.Run("read_only_directory_returns_error", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("running as root, permission test unreliable")
		}
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		// Make directory read-only.
		if err := os.Chmod(dir, 0555); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(dir, 0755) // restore for cleanup

		s := &Settings{}
		err := Save(home, s)
		if err == nil {
			t.Fatal("Save() should fail on read-only directory")
		}
	})

	t.Run("writes_llm_provider_settings", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{
			LLM: llmconfig.Settings{
				DefaultProvider: "primary",
				Providers: map[string]llmconfig.Profile{
					"primary": {
						Protocol:  llmconfig.ProtocolOpenAI,
						BaseURL:   "https://example.test/v1",
						Model:     "test-model",
						Auth:      llmconfig.AuthAPIKey,
						APIKeyEnv: "OPENAI_KEY",
					},
				},
			},
		}
		if err := Save(home, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		data, err := os.ReadFile(filepath.Join(home, ".foxharness", "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			LLM llmconfig.Settings `json:"llm"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed.LLM.DefaultProvider != "primary" {
			t.Fatalf("DefaultProvider = %q, want primary", parsed.LLM.DefaultProvider)
		}
		if parsed.LLM.Providers["primary"].BaseURL != "https://example.test/v1" {
			t.Fatalf("provider = %+v, want saved provider", parsed.LLM.Providers["primary"])
		}
	})

	t.Run("updates_provider_model_and_preserves_unknown_fields", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		original := `{
		  "theme": "dark",
		  "llm": {
		    "future_llm_field": true,
		    "default_provider": "primary",
		    "providers": {
		      "primary": {
		        "protocol": "openai",
		        "base_url": "https://example.test/v1",
		        "model": "old-model",
		        "api_key_env": "OPENAI_KEY",
		        "vendor_extra": "keep"
		      }
		    }
		  }
		}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if err := SetProviderModel(loaded, "primary", "new-model"); err != nil {
			t.Fatalf("SetProviderModel() error = %v", err)
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
			t.Fatalf("theme = %v, want preserved", parsed["theme"])
		}
		llm := parsed["llm"].(map[string]any)
		if llm["future_llm_field"] != true {
			t.Fatalf("future_llm_field = %v, want preserved", llm["future_llm_field"])
		}
		providers := llm["providers"].(map[string]any)
		primary := providers["primary"].(map[string]any)
		if primary["model"] != "new-model" {
			t.Fatalf("model = %v, want new-model", primary["model"])
		}
		if primary["vendor_extra"] != "keep" {
			t.Fatalf("vendor_extra = %v, want preserved", primary["vendor_extra"])
		}
	})

	t.Run("writes_tui_settings", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{
			TUI: TUISettings{
				Theme:      "codex",
				Statusline: []string{"model", "project", "git-branch"},
			},
		}
		if err := Save(home, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		data, err := os.ReadFile(filepath.Join(home, ".foxharness", "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		var parsed struct {
			TUI TUISettings `json:"tui"`
		}
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed.TUI.Theme != "codex" {
			t.Fatalf("theme = %q, want codex", parsed.TUI.Theme)
		}
		wantStatusline := []string{"model", "project", "git-branch"}
		if !reflect.DeepEqual(parsed.TUI.Statusline, wantStatusline) {
			t.Fatalf("statusline = %#v, want %#v", parsed.TUI.Statusline, wantStatusline)
		}
	})

	t.Run("updates_tui_settings_and_preserves_unknown_fields", func(t *testing.T) {
		home := t.TempDir()
		dir := filepath.Join(home, ".foxharness")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		original := `{
		  "future_top": "keep",
		  "theme": "legacy-top-level",
		  "tui": {
		    "future_tui": true,
		    "theme": "amber",
		    "statusline": ["model"]
		  }
		}`
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(original), 0644); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(home)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		loaded.TUI.Theme = "codex"
		loaded.TUI.Statusline = []string{"project", "context-used"}
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
		if parsed["future_top"] != "keep" {
			t.Fatalf("future_top = %v, want preserved", parsed["future_top"])
		}
		if parsed["theme"] != "legacy-top-level" {
			t.Fatalf("top-level theme = %v, want preserved", parsed["theme"])
		}
		tui := parsed["tui"].(map[string]any)
		if tui["future_tui"] != true {
			t.Fatalf("future_tui = %v, want preserved", tui["future_tui"])
		}
		if tui["theme"] != "codex" {
			t.Fatalf("tui.theme = %v, want codex", tui["theme"])
		}
		statusline := tui["statusline"].([]any)
		if got := []any{statusline[0], statusline[1]}; !reflect.DeepEqual(got, []any{"project", "context-used"}) {
			t.Fatalf("tui.statusline = %#v, want [project context-used]", statusline)
		}
	})
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}
