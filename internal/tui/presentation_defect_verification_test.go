package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tui/selector"
	tea "github.com/charmbracelet/bubbletea"
)

func TestUITUI005BlockingOverlaysOwnInputAndFitConstrainedTerminal(t *testing.T) {
	tests := []struct {
		name    string
		marker  string
		install func(*Model)
	}{
		{
			name:   "question",
			marker: "Choose?",
			install: func(m *Model) {
				m.askForm = newAskForm(askRequest{
					questions: []tools.Question{{Prompt: "Choose?", Options: []tools.Option{{Label: "A"}, {Label: "B"}}}},
					reply:     make(chan answerResult, 1),
				})
			},
		},
		{
			name:   "plan review",
			marker: "Plan",
			install: func(m *Model) {
				m.planForm = newPlanReviewForm(planReviewRequest{planMarkdown: "# Plan\n\n1. Inspect\n2. Change", reply: make(chan planReviewResult, 1)})
			},
		},
		{
			name:   "approval",
			marker: "Approve tool call",
			install: func(m *Model) {
				m.approvalForm = newApprovalForm(permissionRequest{
					approval: permission.ApprovalRequest{Request: permission.Request{ToolName: "bash", Action: "printf test", CWD: "/tmp/work"}},
					reply:    make(chan permission.UserDecision, 1),
				})
			},
		},
		{name: "permission", marker: "Permissions", install: func(m *Model) { m.permissionForm = newPermissionForm(permission.Snapshot{}) }},
		{name: "effort", marker: "Effort", install: func(m *Model) { m.effortForm = newEffortForm("openai", []string{"low", "medium", "high"}, "medium") }},
		{
			name:   "rewind",
			marker: "(current)",
			install: func(m *Model) {
				rewind := selector.New(nil)
				m.rewindSelector = &rewind
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(context.Background(), newFakeRunner(), Config{})
			m, _ = update(t, m, tea.WindowSizeMsg{Width: 64, Height: 12})
			m.input = []rune("draft")
			m.inputCursor = len(m.input)
			tc.install(&m)

			m, cmd := update(t, m, keyRunes("x"))
			if cmd != nil {
				t.Fatalf("overlay key returned unexpected command")
			}
			if got := string(m.input); got != "draft" {
				t.Fatalf("overlay leaked key to base input: %q", got)
			}
			assertViewFitsTerminal(t, m)
			if view := stripANSI(m.View()); !strings.Contains(view, tc.marker) {
				t.Fatalf("constrained overlay missing %q:\n%s", tc.marker, view)
			}
		})
	}
}
