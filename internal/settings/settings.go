package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

// Settings holds the user preferences read from ~/.foxharness/settings.json.
// The raw field preserves the original JSON bytes so that Save can rewrite the
// file without dropping unknown fields added by future versions.
type Settings struct {
	Model string
	LLM   llmconfig.Settings
	TUI   TUISettings

	raw json.RawMessage
}

// TUISettings contains persisted terminal UI preferences.
type TUISettings struct {
	Theme      string   `json:"theme,omitempty"`
	Statusline []string `json:"statusline,omitempty"`
}

// Load reads ~/.foxharness/settings.json. If the file is missing, malformed,
// or unreadable, it returns a zero-value Settings without error so callers can
// fall back to defaults transparently.
func Load(homeDir string) (*Settings, error) {
	path := filepath.Join(homeDir, ".foxharness", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return &Settings{}, nil
	}

	raw := json.RawMessage(data)
	var parsed struct {
		Model string             `json:"model"`
		LLM   llmconfig.Settings `json:"llm"`
		TUI   json.RawMessage    `json:"tui"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		log.Printf("[Settings] failed to parse %s: %v", path, err)
		return &Settings{}, nil
	}

	return &Settings{Model: parsed.Model, LLM: parsed.LLM, TUI: parseTUISettings(path, parsed.TUI), raw: raw}, nil
}

func parseTUISettings(path string, raw json.RawMessage) TUISettings {
	if len(raw) == 0 || string(raw) == "null" {
		return TUISettings{}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		log.Printf("[Settings] failed to parse tui settings in %s: %v", path, err)
		return TUISettings{}
	}

	var tui TUISettings
	if rawTheme, ok := fields["theme"]; ok {
		if err := json.Unmarshal(rawTheme, &tui.Theme); err != nil {
			log.Printf("[Settings] ignored invalid tui.theme in %s: %v", path, err)
			tui.Theme = ""
		}
	}
	if rawStatusline, ok := fields["statusline"]; ok {
		items, err := parseTUIStatusline(rawStatusline)
		if err != nil {
			log.Printf("[Settings] ignored invalid tui.statusline in %s: %v", path, err)
		} else {
			tui.Statusline = items
		}
	}
	return tui
}

func parseTUIStatusline(raw json.RawMessage) ([]string, error) {
	var items []string
	if err := json.Unmarshal(raw, &items); err == nil {
		return items, nil
	}

	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return nil, err
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out, nil
}

// Save writes Settings to ~/.foxharness/settings.json atomically. It creates
// the .foxharness directory if needed and preserves any unknown fields already
// present in the file. The output file has 0600 permissions.
func Save(homeDir string, s *Settings) error {
	dir := filepath.Join(homeDir, ".foxharness")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}

	merged := mergeRaw(s)

	tmp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(merged); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}

	dst := filepath.Join(dir, "settings.json")
	if err := os.Rename(tmpPath, dst); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	if err := os.Chmod(dst, 0600); err != nil {
		return fmt.Errorf("chmod settings file: %w", err)
	}

	return nil
}

// SetProviderModel updates the model for an existing LLM provider profile.
func SetProviderModel(s *Settings, providerID string, model string) error {
	if s == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	providerID = strings.TrimSpace(providerID)
	model = strings.TrimSpace(model)
	if providerID == "" {
		return fmt.Errorf("provider id cannot be empty")
	}
	if model == "" {
		return fmt.Errorf("model cannot be empty")
	}
	if s.LLM.Providers == nil {
		return fmt.Errorf("provider profile %q not found", providerID)
	}
	profile, ok := s.LLM.Providers[providerID]
	if !ok {
		return fmt.Errorf("provider profile %q not found", providerID)
	}
	profile.Model = model
	s.LLM.Providers[providerID] = profile
	return nil
}

// SetProvider upserts a provider profile under llm.providers, creating the map
// when none exists. It is the persistence entry point used by the `fox config`
// add flow.
func SetProvider(s *Settings, id string, profile llmconfig.Profile) error {
	if s == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("provider id cannot be empty")
	}
	if s.LLM.Providers == nil {
		s.LLM.Providers = map[string]llmconfig.Profile{}
	}
	s.LLM.Providers[id] = profile
	return nil
}

// SetDefaultProvider sets llm.default_provider after verifying that a profile
// with the given id exists, mirroring SetProviderModel's existence check.
func SetDefaultProvider(s *Settings, id string) error {
	if s == nil {
		return fmt.Errorf("settings cannot be nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("provider id cannot be empty")
	}
	if s.LLM.Providers == nil {
		return fmt.Errorf("provider profile %q not found", id)
	}
	if _, ok := s.LLM.Providers[id]; !ok {
		return fmt.Errorf("provider profile %q not found", id)
	}
	s.LLM.DefaultProvider = id
	return nil
}

// mergeRaw builds the final JSON bytes. If raw bytes from a previous load
// exist, it patches known LLM fields while preserving unknown fields and
// legacy top-level fields. Otherwise it marshals the new settings schema from
// scratch.
func mergeRaw(s *Settings) []byte {
	if s.raw != nil {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(s.raw, &m); err == nil {
			if hasLLMSettings(s.LLM) {
				m["llm"] = mergeLLMRaw(m["llm"], s.LLM)
			}
			if hasTUISettings(s.TUI) {
				m["tui"] = mergeTUIRaw(m["tui"], s.TUI)
			}
			out, err := json.MarshalIndent(m, "", "  ")
			if err == nil {
				return append(out, '\n')
			}
		}
	}

	m := map[string]any{}
	if hasLLMSettings(s.LLM) {
		m["llm"] = s.LLM
	}
	if hasTUISettings(s.TUI) {
		m["tui"] = s.TUI
	}
	out, _ := json.MarshalIndent(m, "", "  ")
	return append(out, '\n')
}

func hasLLMSettings(llm llmconfig.Settings) bool {
	return llm.DefaultProvider != "" || len(llm.Providers) > 0
}

func hasTUISettings(tui TUISettings) bool {
	return tui.Theme != "" || len(tui.Statusline) > 0
}

func mergeLLMRaw(raw json.RawMessage, llm llmconfig.Settings) json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	if m == nil {
		m = make(map[string]json.RawMessage)
	}
	if llm.DefaultProvider != "" {
		defaultProviderJSON, _ := json.Marshal(llm.DefaultProvider)
		m["default_provider"] = defaultProviderJSON
	}
	if len(llm.Providers) > 0 {
		m["providers"] = mergeProvidersRaw(m["providers"], llm.Providers)
	}
	out, _ := json.Marshal(m)
	return out
}

func mergeTUIRaw(raw json.RawMessage, tui TUISettings) json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	if m == nil {
		m = make(map[string]json.RawMessage)
	}
	setStringField(m, "theme", tui.Theme)
	if len(tui.Statusline) > 0 {
		statuslineJSON, _ := json.Marshal(tui.Statusline)
		m["statusline"] = statuslineJSON
	}
	out, _ := json.Marshal(m)
	return out
}

func mergeProvidersRaw(raw json.RawMessage, providers map[string]llmconfig.Profile) json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	if m == nil {
		m = make(map[string]json.RawMessage)
	}
	for id, profile := range providers {
		m[id] = mergeProviderRaw(m[id], profile)
	}
	out, _ := json.Marshal(m)
	return out
}

func mergeProviderRaw(raw json.RawMessage, profile llmconfig.Profile) json.RawMessage {
	var m map[string]json.RawMessage
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &m)
	}
	if m == nil {
		m = make(map[string]json.RawMessage)
	}
	setStringField(m, "protocol", profile.Protocol)
	setStringField(m, "base_url", profile.BaseURL)
	setStringField(m, "model", profile.Model)
	setStringField(m, "auth", profile.Auth)
	setStringField(m, "api_key_env", profile.APIKeyEnv)
	setStringField(m, "api_key", profile.APIKey)
	out, _ := json.Marshal(m)
	return out
}

func setStringField(m map[string]json.RawMessage, name string, value string) {
	if value == "" {
		return
	}
	data, _ := json.Marshal(value)
	m[name] = data
}
