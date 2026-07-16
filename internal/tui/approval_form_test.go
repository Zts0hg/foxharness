package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
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
