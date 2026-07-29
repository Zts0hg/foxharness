package tui

import (
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	tea "github.com/charmbracelet/bubbletea"
)

type approvalDoneMsg struct{}

type approvalForm struct {
	req      permissionRequest
	action   int
	feedback []rune
	done     bool
}

func newApprovalForm(req permissionRequest) *approvalForm {
	return &approvalForm{req: req}
}

func (f *approvalForm) update(msg tea.KeyMsg) tea.Cmd {
	if f.done {
		return nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		f.action = 2
		return f.submit()
	case tea.KeyTab, tea.KeyRight, tea.KeyDown:
		f.action = (f.action + 1) % 4
	case tea.KeyShiftTab, tea.KeyLeft, tea.KeyUp:
		f.action = (f.action + 3) % 4
	case tea.KeyBackspace, tea.KeyDelete:
		if f.action == 3 && len(f.feedback) > 0 {
			f.feedback = f.feedback[:len(f.feedback)-1]
		}
	case tea.KeySpace:
		if f.action == 3 {
			f.feedback = append(f.feedback, ' ')
		}
	case tea.KeyRunes:
		if f.action == 3 {
			f.feedback = append(f.feedback, msg.Runes...)
		}
	case tea.KeyEnter:
		return f.submit()
	}
	return nil
}

func (f *approvalForm) submit() tea.Cmd {
	f.done = true
	return func() tea.Msg { return approvalDoneMsg{} }
}

func (f *approvalForm) decision() permission.UserDecision {
	switch f.action {
	case 0:
		return permission.UserDecision{Kind: permission.UserAllowOnce}
	case 1:
		return permission.UserDecision{Kind: permission.UserAllowSession}
	case 3:
		return permission.UserDecision{Kind: permission.UserDenyFeedback, Feedback: strings.TrimSpace(string(f.feedback))}
	default:
		return permission.UserDecision{Kind: permission.UserDeny}
	}
}

func (f *approvalForm) view(width int) string {
	req := f.req.approval.Request
	contentWidth := max(width-inputStyle.GetHorizontalFrameSize()-2, 20)
	var b strings.Builder
	b.WriteString(headerStyle.Render("Approve tool call"))
	b.WriteString("\n\n")
	b.WriteString(wrapText("Action: "+req.Action, contentWidth) + "\n")
	b.WriteString(wrapText("CWD: "+req.CWD, contentWidth) + "\n")
	b.WriteString(wrapText("Scope: exact invocation in this session", contentWidth) + "\n")
	b.WriteString(fmt.Sprintf("Risk: %s\n", req.Risk))
	if req.Capabilities.Behavior == toolpolicy.BehaviorHumanOnly && req.Capabilities.Reason != "" {
		b.WriteString(wrapText("Policy: "+req.Capabilities.Reason, contentWidth) + "\n")
	}
	if f.req.approval.Review != nil {
		b.WriteString(wrapText("Review: "+f.req.approval.Review.Rationale, contentWidth) + "\n")
	}
	if f.req.approval.ReviewerFailure != "" {
		b.WriteString("Auto-review unavailable after three attempts.\n")
		b.WriteString(wrapText("Reviewer failure: "+f.req.approval.ReviewerFailure, contentWidth) + "\n")
	}
	b.WriteString("\n")
	labels := []string{"Yes", "Yes, session", "No", "No + feedback"}
	for i, label := range labels {
		b.WriteString(f.renderAction(i, label))
		b.WriteString("\n")
	}
	if f.action == 3 {
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render("Feedback: "))
		if len(f.feedback) == 0 {
			b.WriteString(placeholderStyle.Render("Tell the agent what to do instead"))
		} else {
			b.WriteString(string(f.feedback))
		}
		b.WriteString(cursorStyle.Render("▏"))
	}
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("Tab/↑/↓ choose · Enter confirm · Esc deny"))
	lines := strings.Split(b.String(), "\n")
	for i := range lines {
		lines[i] = fitLine(lines[i], contentWidth)
	}
	return strings.Join(lines, "\n")
}

func (f *approvalForm) renderAction(index int, label string) string {
	if f.action == index {
		return askFocusedStyle.Render("❯ " + label)
	}
	return "  " + label
}
