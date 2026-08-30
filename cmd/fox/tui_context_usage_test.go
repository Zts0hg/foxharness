package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/tui"
)

/* TestTUIInteractiveContextUsageReflectsEngineEstimate pins the baseline
 * sidebar behavior: the shown context usage is the live engine estimate,
 * which includes the tool-definition overhead and the full projection, not a
 * transcript-only re-estimate. */
func TestTUIInteractiveContextUsageReflectsEngineEstimate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	workDir := t.TempDir()
	instructions := strings.Repeat("TARGET_CONTEXT_USAGE_INSTRUCTION line\n", 4000)
	if err := os.WriteFile(filepath.Join(workDir, "AGENTS.md"), []byte(instructions), 0o644); err != nil {
		t.Fatal(err)
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), foxConfig{
		WorkDir: workDir, Model: "tui-model", MaxTurns: 4,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
	}, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: cancelPlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return &targetTUIProvider{}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer startup.Close(context.Background())

	before := startup.Application.State().ContextUsage
	outcome, runErr := startup.Application.Run(context.Background(), app.RunCommand{Prompt: "usage probe"}, nil)
	if runErr != nil || outcome == nil {
		t.Fatalf("run = %#v/%v", outcome, runErr)
	}
	after := startup.Application.State().ContextUsage
	if contextUsageRank(after) <= contextUsageRank(before) {
		t.Fatalf("context usage after a run = %q, want the live engine estimate above the idle %q", after, before)
	}
	if contextUsageRank(after) < 10 {
		t.Fatalf("context usage after a run = %q, want the tool-overhead-inclusive projection estimate", after)
	}
}

/* contextUsageRank orders the sidebar usage labels: "0%" < "<1%" < "N%". */
func contextUsageRank(label string) int {
	if strings.HasPrefix(label, "<") {
		return 1
	}
	value, _ := strconv.Atoi(strings.TrimSuffix(label, "%"))
	return value
}
