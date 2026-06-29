package llmresolve

import (
	"fmt"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/settings"
)

// FromUserSettings resolves LLM configuration from ~/.foxharness/settings.json,
// environment overrides, and optional CLI overrides.
func FromUserSettings(homeDir string, cli llmconfig.CLIOverrides, lookup llmconfig.EnvLookup) (llmconfig.ResolvedConfig, error) {
	loaded, err := settings.Load(homeDir)
	if err != nil {
		return llmconfig.ResolvedConfig{}, err
	}
	env := llmconfig.EnvOverridesFromLookup(lookup)
	resolved, err := llmconfig.Resolve(loaded.LLM, env, cli, lookup)
	if err != nil {
		return llmconfig.ResolvedConfig{}, fmt.Errorf("resolve LLM configuration: %w", err)
	}
	return resolved, nil
}
