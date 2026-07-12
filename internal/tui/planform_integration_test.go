package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

func TestModelRendersPlanReviewInlineAndReturnsRevision(t *testing.T) {
	runner := newFakeRunner()
	reviewer := NewPlanReviewer()
	m := NewModel(context.Background(), runner, Config{PlanReviewer: reviewer})
	m.entries = nil
	m.appendEntry("assistant", "", "TRANSCRIPT_DURING_PLAN_REVIEW", false)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 32})

	plan := "# Exact plan\n\n- first\n- second"
	reply := make(chan planReviewResult, 1)
	req := planReviewRequest{planMarkdown: plan, reply: reply}
	m, _ = update(t, m, planReviewMsg{req: req})
	if m.planForm == nil || m.planForm.req.planMarkdown != plan {
		t.Fatalf("plan form = %#v, want exact source", m.planForm)
	}
	view := stripANSI(m.View())
	for _, want := range []string{"TRANSCRIPT_DURING_PLAN_REVIEW", "Exact plan", "Approve", "Continue planning"} {
		if !strings.Contains(view, want) {
			t.Fatalf("plan review view missing %q:\n%s", want, view)
		}
	}

	m, _ = update(t, m, key(tea.KeyTab))
	m, _ = update(t, m, keyRunes("Add rollback verification."))
	if len(m.input) != 0 {
		t.Fatalf("plan feedback leaked to prompt input: %q", string(m.input))
	}
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("revision did not return completion command")
	}
	done := cmd()
	if _, ok := done.(planReviewDoneMsg); !ok {
		t.Fatalf("completion message = %T, want planReviewDoneMsg", done)
	}
	m, rearm := update(t, m, done)
	if m.planForm != nil || rearm == nil {
		t.Fatalf("plan form not cleared/re-armed: form=%#v cmd=%v", m.planForm, rearm)
	}
	select {
	case result := <-reply:
		if result.cancelled || result.review.Decision != tools.PlanContinuePlanning || result.review.Feedback != "Add rollback verification." {
			t.Fatalf("revision result = %#v", result)
		}
	default:
		t.Fatal("model did not reply to plan reviewer")
	}
}

func TestModelPlanApprovalResetsSelectedModeAndContinuesRun(t *testing.T) {
	runner := newFakeRunner()
	runner.collaborationMode = collaboration.ModeFormalPlan
	reviewer := NewPlanReviewer()
	m := NewModel(context.Background(), runner, Config{PlanReviewer: reviewer})
	m.running = true

	reply := make(chan planReviewResult, 1)
	m, _ = update(t, m, planReviewMsg{req: planReviewRequest{planMarkdown: "# Plan", reply: reply}})
	m, cmd := update(t, m, keyEnter())
	m, _ = update(t, m, cmd())

	if m.collaborationMode != collaboration.ModeDefault || runner.collaborationMode != collaboration.ModeDefault {
		t.Fatalf("approval mode: model=%q runner=%q, want Default", m.collaborationMode, runner.collaborationMode)
	}
	if !m.running {
		t.Fatal("approval ended the active run instead of continuing it")
	}
	select {
	case result := <-reply:
		if result.review.Decision != tools.PlanApproved || result.cancelled {
			t.Fatalf("approval result = %#v", result)
		}
	default:
		t.Fatal("model did not reply with approval")
	}
}

func TestModelPlanReviewCancelKeepsFormalMode(t *testing.T) {
	runner := newFakeRunner()
	runner.collaborationMode = collaboration.ModeFormalPlan
	m := NewModel(context.Background(), runner, Config{PlanReviewer: NewPlanReviewer()})
	reply := make(chan planReviewResult, 1)
	m, _ = update(t, m, planReviewMsg{req: planReviewRequest{planMarkdown: "# Plan", reply: reply}})
	m, cmd := update(t, m, key(tea.KeyEsc))
	m, _ = update(t, m, cmd())

	if !m.collaborationMode.PlanEnabled() || !runner.collaborationMode.PlanEnabled() {
		t.Fatalf("cancel left Formal mode: model=%q runner=%q", m.collaborationMode, runner.collaborationMode)
	}
	select {
	case result := <-reply:
		if !result.cancelled {
			t.Fatalf("cancel result = %#v", result)
		}
	default:
		t.Fatal("model did not reply on cancellation")
	}
}
