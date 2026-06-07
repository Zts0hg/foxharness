package keeprun

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// configFileName is the project-root file that overrides keep-run defaults.
const configFileName = "keep-run.config.json"

// Default values for keep-run configuration, matching spec FR-008.
const (
	defaultReviewMode      = "subagent"
	defaultBackoff         = "exponential"
	defaultClarifyPrompt   = "Make decisions that prioritize correctness, simplicity, and alignment with project conventions."
	defaultReviewFixPrompt = "Fix all issues, warnings, and suggestions. Prioritize correctness and code quality. Follow project constitution and TDD principles."
)

// Config holds the keep-run pipeline configuration loaded from
// keep-run.config.json. See spec FR-008 for field semantics and defaults.
type Config struct {
	RemoteEnabled   bool        `json:"remote_enabled"`
	ReviewMode      string      `json:"review_mode"`
	ClarifyPrompt   string      `json:"clarify_prompt"`
	ReviewFixPrompt string      `json:"review_fix_prompt"`
	RetryPolicy     RetryPolicy `json:"retry_policy"`
}

// RetryPolicy controls backoff behavior for error recovery during the pipeline.
type RetryPolicy struct {
	Backoff string `json:"backoff"`
}

// DefaultConfig returns the configuration used when no config file exists or a
// field is omitted. The values mirror the defaults table in spec FR-008.
func DefaultConfig() Config {
	return Config{
		RemoteEnabled:   true,
		ReviewMode:      defaultReviewMode,
		ClarifyPrompt:   defaultClarifyPrompt,
		ReviewFixPrompt: defaultReviewFixPrompt,
		RetryPolicy:     RetryPolicy{Backoff: defaultBackoff},
	}
}

// LoadConfig reads keep-run.config.json from dir and returns the resulting
// configuration with defaults applied for any missing field.
//
// A missing file is not an error: DefaultConfig is returned. Malformed JSON
// that cannot be parsed returns an error along with DefaultConfig so callers may
// still fall back if they choose. Because Go cannot distinguish an omitted bool
// from an explicit false, fields are decoded through pointers so that an
// explicit "remote_enabled": false is preserved while an omitted field keeps its
// default. Empty-string overrides are ignored in favor of the defaults.
func LoadConfig(dir string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read %s: %w", configFileName, err)
	}

	var raw struct {
		RemoteEnabled   *bool   `json:"remote_enabled"`
		ReviewMode      *string `json:"review_mode"`
		ClarifyPrompt   *string `json:"clarify_prompt"`
		ReviewFixPrompt *string `json:"review_fix_prompt"`
		RetryPolicy     *struct {
			Backoff *string `json:"backoff"`
		} `json:"retry_policy"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return DefaultConfig(), fmt.Errorf("parse %s: %w", configFileName, err)
	}

	if raw.RemoteEnabled != nil {
		cfg.RemoteEnabled = *raw.RemoteEnabled
	}
	if raw.ReviewMode != nil && *raw.ReviewMode != "" {
		cfg.ReviewMode = *raw.ReviewMode
	}
	if raw.ClarifyPrompt != nil && *raw.ClarifyPrompt != "" {
		cfg.ClarifyPrompt = *raw.ClarifyPrompt
	}
	if raw.ReviewFixPrompt != nil && *raw.ReviewFixPrompt != "" {
		cfg.ReviewFixPrompt = *raw.ReviewFixPrompt
	}
	if raw.RetryPolicy != nil && raw.RetryPolicy.Backoff != nil && *raw.RetryPolicy.Backoff != "" {
		cfg.RetryPolicy.Backoff = *raw.RetryPolicy.Backoff
	}

	return cfg, nil
}
