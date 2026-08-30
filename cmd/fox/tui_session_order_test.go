package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/tui"
)

/* TestTUIRuntimeSelectsSessionBeforeProviderConstruction pins the baseline
 * admission order on the interactive entry: a bad session selection is
 * reported even when the LLM configuration is under-specified. */
func TestTUIRuntimeSelectsSessionBeforeProviderConstruction(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	config := foxConfig{
		WorkDir: workDir, SessionID: "missing-session", Model: "tui-model",
		ResolvedLLM: llmconfig.ResolvedConfig{},
	}
	_, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return &targetTUIProvider{}, nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "Session missing-session 不存在") {
		t.Fatalf("newTUIStartupWithProviderFactory() error = %v, want the session selection error", err)
	}
}
