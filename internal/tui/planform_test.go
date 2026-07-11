package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

func planFormFor(plan string) *planReviewForm {
	return newPlanReviewForm(planReviewRequest{
		planMarkdown: plan,
		reply:        make(chan planReviewResult, 1),
	})
}

func TestPlanReviewFormRetainsExactSourceAndApproves(t *testing.T) {
	plan := "\n# Exact proposal\n\nNo trailing newline"
	form := planFormFor(plan)
	if form.req.planMarkdown != plan {
		t.Fatalf("form source = %q, want exact %q", form.req.planMarkdown, plan)
	}

	cmd := form.update(key(tea.KeyEnter))
	if !form.done || form.cancelled || form.review.Decision != tools.PlanApproved {
		t.Fatalf("approve state: done=%v cancelled=%v review=%#v", form.done, form.cancelled, form.review)
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
	if form.review.Decision != tools.PlanContinuePlanning || form.review.Feedback != "Please split the migration." {
		t.Fatalf("review = %#v", form.review)
	}
	if cmd == nil {
		t.Fatal("continue command is nil")
	}

	empty := planFormFor("# Proposal")
	empty.update(key(tea.KeyTab))
	empty.update(key(tea.KeyEnter))
	if empty.review.Decision != tools.PlanContinuePlanning || empty.review.Feedback != "" {
		t.Fatalf("empty-feedback review = %#v", empty.review)
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
	if !form.done || !form.cancelled || form.review.Decision != "" {
		t.Fatalf("cancel state: done=%v cancelled=%v review=%#v", form.done, form.cancelled, form.review)
	}
	if cmd == nil {
		t.Fatal("cancel command is nil")
	}
}
