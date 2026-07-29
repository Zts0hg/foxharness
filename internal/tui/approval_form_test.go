package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalFormShowsHumanOnlyPolicyReason(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: permission.ApprovalRequest{Request: permission.Request{
		Action: "custom_tool", CWD: "/tmp/work", Risk: permission.RiskHigh,
		Capabilities: toolpolicy.Assessment{
			Behavior: toolpolicy.BehaviorHumanOnly,
			Reason:   "registered tool does not declare permission capabilities",
		},
	}}})

	view := form.view(100)
	if !strings.Contains(view, "Policy: registered tool does not declare permission capabilities") {
		t.Fatalf("approval view does not explain human-only routing:\n%s", view)
	}
}

func TestApprovalFormWrapsExactActionInsteadOfTruncatingIt(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: permission.ApprovalRequest{Request: permission.Request{
		Action: "skill inspect; planned commands=[touch first_marker && rm -rf final_marker]",
		CWD:    "/tmp/work",
		Risk:   permission.RiskCritical,
	}}})

	view := form.view(36)
	if !strings.Contains(view, "final_marker") {
		t.Fatalf("approval view truncated the exact action:\n%s", view)
	}
}

func TestApprovalFormVerticalArrowsMoveBetweenOptions(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: permission.ApprovalRequest{Request: permission.Request{
		Action: "bash date",
		CWD:    "/tmp/work",
		Risk:   permission.RiskLow,
	}}})

	form.update(tea.KeyMsg{Type: tea.KeyDown})
	if form.action != 1 {
		t.Fatalf("action after down = %d, want 1", form.action)
	}
	form.update(tea.KeyMsg{Type: tea.KeyUp})
	if form.action != 0 {
		t.Fatalf("action after up = %d, want 0", form.action)
	}
	form.update(tea.KeyMsg{Type: tea.KeyUp})
	if form.action != 3 {
		t.Fatalf("action after wrap up = %d, want 3", form.action)
	}
}
