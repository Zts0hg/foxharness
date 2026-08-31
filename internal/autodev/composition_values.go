package autodev

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveModel applies Autodev configuration precedence over the CLI model.
func ResolveModel(cliModel string, config AutodevConfig) string {
	if config.Model != "" {
		return config.Model
	}
	return cliModel
}

// ResolveEngineerPersona loads the configured Engineer persona relative to the repository root.
func ResolveEngineerPersona(config AutodevConfig, repoRoot string) (string, error) {
	if strings.TrimSpace(config.EngineerPrompt) != "" {
		return config.EngineerPrompt, nil
	}
	if config.EngineerPromptFile == "" {
		return "", nil
	}
	path := config.EngineerPromptFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(repoRoot, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read engineer_prompt_file: %w", err)
	}
	return string(data), nil
}
