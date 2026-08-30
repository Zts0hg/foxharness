package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/tui"
)

/* TestTUIRestrictedFormalPlanRunRequiresSubmitPlan verifies that an
 * allowed-tools restriction missing a required Formal Plan lifecycle tool
 * fails before model invocation, listing every missing tool. */
func TestTUIRestrictedFormalPlanRunRequiresSubmitPlan(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := &targetTUIProvider{}
	startup := newFormalPlanRestrictionStartup(t, model)
	defer func() {
		if err := startup.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	outcome, err := startup.Application.Run(context.Background(), app.RunCommand{
		Prompt:            "plan then implement",
		CollaborationMode: "formal_plan",
		AllowedTools:      []string{"read_file", "bash", "ask_user_question", "update_todo"},
	}, nil)
	if err == nil {
		t.Fatalf("restricted Formal Plan run without submit_plan succeeded: %#v", outcome)
	}
	if !strings.Contains(err.Error(), "restricted Formal Plan run is missing required lifecycle tools: submit_plan") {
		t.Fatalf("restriction error = %v, want the missing submit_plan lifecycle error", err)
	}

	partial, err := startup.Application.Run(context.Background(), app.RunCommand{
		Prompt: "plan then implement", CollaborationMode: "formal_plan",
		AllowedTools: []string{"read_file"},
	}, nil)
	if err == nil {
		t.Fatalf("restricted Formal Plan run missing four lifecycle tools succeeded: %#v", partial)
	}
	if !strings.Contains(err.Error(), "restricted Formal Plan run is missing required lifecycle tools: bash, ask_user_question, submit_plan, update_todo") {
		t.Fatalf("restriction error = %v, want every missing lifecycle tool", err)
	}
}

/* TestTUIRestrictedFormalPlanRunWithFullLifecycleToolsetRuns verifies that a
 * restriction carrying the complete lifecycle tool set is accepted and the
 * run completes through the Formal Plan lifecycle. */
func TestTUIRestrictedFormalPlanRunWithFullLifecycleToolsetRuns(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	model := &formalPlanTUIProvider{}
	startup := newFormalPlanRestrictionStartup(t, model)
	defer func() {
		if err := startup.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}()

	outcome, err := startup.Application.Run(context.Background(), app.RunCommand{
		Prompt: "plan then implement", CollaborationMode: "formal_plan",
		AllowedTools: []string{"read_file", "bash", "ask_user_question", "submit_plan", "update_todo", "write_file"},
	}, nil)
	if err != nil || outcome == nil || outcome.FinalMessage != "implemented" {
		t.Fatalf("restricted Formal Plan run = %#v/%v, want completion", outcome, err)
	}
}

/* newFormalPlanRestrictionStartup builds one TUI startup for restriction tests. */
func newFormalPlanRestrictionStartup(t *testing.T, model provider.LLMProvider) tui.Startup {
	t.Helper()
	config := foxConfig{
		WorkDir: t.TempDir(), Model: "tui-model", MaxTurns: 6,
		ResolvedLLM: llmconfig.ResolvedConfig{Protocol: "openai", BaseURL: "https://example.test", Model: "tui-model"},
	}
	startup, err := newTUIStartupWithProviderFactory(context.Background(), config, tui.Interactions{
		Permissions: denyPermissionPort{}, Questions: cancelQuestionPort{}, PlanReview: approvePlanPort{},
	}, func(llmconfig.ResolvedConfig) (provider.LLMProvider, error) { return model, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	startup.Application.ActivateFullAccess(context.Background(), app.FullAccessCommand{})
	return startup
}
