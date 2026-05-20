package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
}

// ---------------------------------------------------------------------------
// Save tests (Task 1.4)
// ---------------------------------------------------------------------------

func TestSave(t *testing.T) {
	t.Run("creates_directory_if_missing", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{Model: "glm-4-plus"}
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
		s := &Settings{Model: "glm-4-plus"}
		if err := Save(home, s); err != nil {
			t.Fatalf("Save() error = %v", err)
		}
		path := filepath.Join(home, ".foxharness", "settings.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]string
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if parsed["model"] != "glm-4-plus" {
			t.Fatalf("model = %q, want glm-4-plus", parsed["model"])
		}
	})

	t.Run("file_permissions_0600", func(t *testing.T) {
		home := t.TempDir()
		s := &Settings{Model: "glm-4-plus"}
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

		// Load, update model, save.
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
		if parsed["model"] != "glm-4-plus" {
			t.Fatalf("model = %v, want glm-4-plus", parsed["model"])
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
		s := &Settings{Model: "glm-4-plus"}
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

		s := &Settings{Model: "glm-4-plus"}
		err := Save(home, s)
		if err == nil {
			t.Fatal("Save() should fail on read-only directory")
		}
	})
}

// ---------------------------------------------------------------------------
// ResolveModel tests (Task 1.6)
// ---------------------------------------------------------------------------

func TestResolveModel(t *testing.T) {
	defaultModel := "glm-4.5-air"

	cases := []struct {
		name     string
		cliFlag  string
		envVar   string
		settings *Settings
		want     string
	}{
		{"all_empty_uses_default", "", "", &Settings{}, defaultModel},
		{"nil_settings_uses_default", "", "", nil, defaultModel},
		{"settings_model_used", "", "", &Settings{Model: "glm-4-plus"}, "glm-4-plus"},
		{"cli_flag_wins_over_all", "glm-4-flash", "glm-4-air", &Settings{Model: "glm-4-plus"}, "glm-4-flash"},
		{"env_var_wins_over_settings", "", "glm-4-flash", &Settings{Model: "glm-4-plus"}, "glm-4-flash"},
		{"cli_flag_wins_over_env", "glm-4-flash", "glm-4-air", &Settings{}, "glm-4-flash"},
		{"empty_settings_model_falls_back", "", "", &Settings{Model: ""}, defaultModel},
		{"cli_flag_only", "glm-4-flash", "", nil, "glm-4-flash"},
		{"env_var_only", "", "glm-4-flash", nil, "glm-4-flash"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveModel(tc.cliFlag, tc.envVar, defaultModel, tc.settings)
			if got != tc.want {
				t.Fatalf("ResolveModel(%q, %q, %q, %+v) = %q, want %q",
					tc.cliFlag, tc.envVar, defaultModel, tc.settings, got, tc.want)
			}
		})
	}
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
