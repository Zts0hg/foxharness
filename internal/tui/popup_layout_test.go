package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/permission"
)

func TestPopupActionOptionsRenderVertically(t *testing.T) {
	t.Run("full access warning", func(t *testing.T) {
		form := newFullAccessWarningForm(permission.Snapshot{})
		view := stripANSI(form.view(100))
		assertNotSameLine(t, view, "Confirm for this session", "Confirm and remember")
		assertNotSameLine(t, view, "Confirm and remember", "Cancel")
	})

	t.Run("tool approval", func(t *testing.T) {
		form := newApprovalForm(permissionRequest{
			approval: permission.ApprovalRequest{
				Request: permission.Request{
					ToolName: "bash",
					Action:   "bash git status --short",
					CWD:      "/tmp/work",
					Risk:     permission.RiskMedium,
				},
			},
		})
		view := stripANSI(form.view(100))
		assertOptionLinesDistinct(t, view, "Yes", "Yes, session")
		assertOptionLinesDistinct(t, view, "Yes, session", "No")
		assertOptionLinesDistinct(t, view, "No", "No + feedback")
	})

	t.Run("plan review", func(t *testing.T) {
		form := planFormFor("# Plan")
		view := stripANSI(form.view(100, 12))
		assertNotSameLine(t, view, "Approve", "Continue planning")
	})
}

func assertOptionLinesDistinct(t *testing.T, text, left, right string) {
	t.Helper()
	leftLine := optionLineIndex(text, left)
	rightLine := optionLineIndex(text, right)
	if leftLine < 0 || rightLine < 0 {
		t.Fatalf("missing option line(s) %q=%d %q=%d:\n%s", left, leftLine, right, rightLine, text)
	}
	if leftLine == rightLine {
		t.Fatalf("%q and %q rendered on same line:\n%s", left, right, text)
	}
}

func optionLineIndex(text, label string) int {
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		trimmed = strings.TrimPrefix(trimmed, "❯ ")
		trimmed = strings.TrimSpace(trimmed)
		if trimmed == label {
			return i
		}
	}
	return -1
}

func assertNotSameLine(t *testing.T, text, left, right string) {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, left) && strings.Contains(line, right) {
			t.Fatalf("%q and %q rendered on same line:\n%s", left, right, text)
		}
	}
}
