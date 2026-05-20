package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// Settings holds the user preferences read from ~/.foxharness/settings.json.
// The raw field preserves the original JSON bytes so that Save can rewrite the
// file without dropping unknown fields added by future versions.
type Settings struct {
	Model string

	raw json.RawMessage
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
		Model string `json:"model"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		log.Printf("[Settings] failed to parse %s: %v", path, err)
		return &Settings{}, nil
	}

	return &Settings{Model: parsed.Model, raw: raw}, nil
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

// ResolveModel picks the effective model from a four-level priority cascade:
//
//	1. cliFlag   (CLI --model argument)
//	2. envVar    (FOX_MODEL environment variable)
//	3. s.Model   (settings.json value)
//	4. fallback  (built-in default passed by caller)
func ResolveModel(cliFlag, envVar, fallback string, s *Settings) string {
	if cliFlag != "" {
		return cliFlag
	}
	if envVar != "" {
		return envVar
	}
	if s != nil && s.Model != "" {
		return s.Model
	}
	return fallback
}

// mergeRaw builds the final JSON bytes. If raw bytes from a previous load
// exist, it patches the model field in-place to preserve unknown fields and
// formatting. Otherwise it marshals from scratch.
func mergeRaw(s *Settings) []byte {
	if s.raw != nil {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(s.raw, &m); err == nil {
			modelJSON, _ := json.Marshal(s.Model)
			m["model"] = modelJSON
			out, err := json.MarshalIndent(m, "", "  ")
			if err == nil {
				return append(out, '\n')
			}
		}
	}

	out, _ := json.MarshalIndent(map[string]string{"model": s.Model}, "", "  ")
	return append(out, '\n')
}
