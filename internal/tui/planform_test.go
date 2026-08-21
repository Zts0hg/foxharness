package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func planFormFor(plan string) *planReviewForm {
	return newPlanReviewForm(planReviewRequest{
		request: app.PlanReviewRequest{PlanMarkdown: plan},
		reply:   make(chan planReviewResult, 1),
	})
}

func TestPlanReviewFormRetainsExactSourceAndApproves(t *testing.T) {
	plan := "\n# Exact proposal\n\nNo trailing newline"
	form := planFormFor(plan)
	if form.req.request.PlanMarkdown != plan {
		t.Fatalf("form source = %q, want exact %q", form.req.request.PlanMarkdown, plan)
	}

	cmd := form.update(key(tea.KeyEnter))
	if !form.done || form.cancelled || form.response.Decision != app.PlanApproved {
		t.Fatalf("approve state: done=%v cancelled=%v response=%#v", form.done, form.cancelled, form.response)
	}
	if cmd == nil {
		t.Fatal("approve command is nil")
	}
	if _, ok := cmd().(planReviewDoneMsg); !ok {
		t.Fatalf("approve command returned %T, want planReviewDoneMsg", cmd())
	}
}

func TestPlanReviewFormContinuesWithOptionalFeedback(t *testing.T) {
	form := planFormFor("# Proposal")
	form.update(key(tea.KeyTab))
	form.update(runes("Please split the migration."))
	cmd := form.update(key(tea.KeyEnter))

	if !form.done || form.cancelled {
		t.Fatalf("continue state: done=%v cancelled=%v", form.done, form.cancelled)
	}
	if form.response.Decision != app.PlanContinuePlanning || form.response.Feedback != "Please split the migration." {
		t.Fatalf("response = %#v", form.response)
	}
	if cmd == nil {
		t.Fatal("continue command is nil")
	}

	empty := planFormFor("# Proposal")
	empty.update(key(tea.KeyTab))
	empty.update(key(tea.KeyEnter))
	if empty.response.Decision != app.PlanContinuePlanning || empty.response.Feedback != "" {
		t.Fatalf("empty-feedback response = %#v", empty.response)
	}
}

func TestPlanReviewFormScrollsRenderedMarkdown(t *testing.T) {
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "- plan line " + strings.Repeat("x", i%5)
	}
	form := planFormFor(strings.Join(lines, "\n"))
	before := stripANSI(form.view(72, 12))
	form.update(key(tea.KeyPgDown))
	after := stripANSI(form.view(72, 12))

	if form.scroll == 0 {
		t.Fatal("PgDown did not advance plan scroll")
	}
	if before == after {
		t.Fatalf("scrolled view did not change:\n%s", after)
	}
}

func TestPlanReviewFormCancelDoesNotApprove(t *testing.T) {
	form := planFormFor("# Proposal")
	cmd := form.update(key(tea.KeyEsc))
	if !form.done || !form.cancelled || form.response.Decision != "" {
		t.Fatalf("cancel state: done=%v cancelled=%v response=%#v", form.done, form.cancelled, form.response)
	}
	if cmd == nil {
		t.Fatal("cancel command is nil")
	}
}
