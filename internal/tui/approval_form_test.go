package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

func TestApprovalFormShowsHumanOnlyPolicyReason(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: app.PermissionRequest{
		Action: "custom_tool", CWD: "/tmp/work", Risk: "high",
		PolicyReason: "registered tool does not declare permission capabilities",
	}})

	view := form.view(100)
	if !strings.Contains(view, "Policy") || !strings.Contains(view, "registered tool does not declare permission capabilities") {
		t.Fatalf("approval view does not explain human-only routing:\n%s", view)
	}
}

func TestApprovalFormUsesCodexLikeGroupedPrompt(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: app.PermissionRequest{
		ToolName: "bash",
		Action:   "bash sed -n '60,240p' src/platform/wechat-ad-service.ts",
		CWD:      "/tmp/work",
		Risk:     "medium",
	}})

	view := stripANSI(form.view(100))
	for _, want := range []string{
		"Would you like to run the following command?",
		"Command",
		"$ sed -n '60,240p' src/platform/wechat-ad-service.ts",
		"Directory",
		"Scope",
		"Choose",
		"Yes            Allow once",
		"Yes, session   Remember for this session",
		"No             Deny this request",
		"No + feedback  Deny and tell the agent why",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("approval view missing %q:\n%s", want, view)
		}
	}
}

func TestApprovalFormWrapsExactActionInsteadOfTruncatingIt(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: app.PermissionRequest{
		Action: "skill inspect; planned commands=[touch first_marker && rm -rf final_marker]",
		CWD:    "/tmp/work",
		Risk:   "critical",
	}})

	view := form.view(36)
	if !strings.Contains(view, "final_marker") {
		t.Fatalf("approval view truncated the exact action:\n%s", view)
	}
}

func TestApprovalFormCollapsesUnavailableAutoReviewFailure(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: app.PermissionRequest{
		ToolName:        "bash",
		Action:          "bash date",
		CWD:             "/tmp/work",
		Risk:            "medium",
		ReviewerReason:  "Auto-review was unavailable after three attempts.",
		ReviewerFailure: "unterminated review JSON object",
	}})

	view := stripANSI(form.view(100))
	if strings.Contains(view, "Auto-review was unavailable after three attempts.") {
		t.Fatalf("approval view repeated generic unavailable rationale:\n%s", view)
	}
	if !strings.Contains(view, "Auto-review unavailable") || !strings.Contains(view, "unterminated review JSON object") {
		t.Fatalf("approval view does not show compact auto-review failure:\n%s", view)
	}
}

func TestApprovalFormVerticalArrowsMoveBetweenOptions(t *testing.T) {
	form := newApprovalForm(permissionRequest{approval: app.PermissionRequest{
		Action: "bash date",
		CWD:    "/tmp/work",
		Risk:   "low",
	}})

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
