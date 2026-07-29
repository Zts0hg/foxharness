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
	if req.Risk != "" {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  Risk: %s", req.Risk)))
	}
	b.WriteString("\n\n")
	b.WriteString(wrapText(approvalPrompt(req), contentWidth) + "\n\n")
	f.writeField(&b, approvalActionTitle(req), approvalActionValue(req), contentWidth)
	f.writeField(&b, "Directory", req.CWD, contentWidth)
	f.writeField(&b, "Scope", "Exact invocation in this session", contentWidth)
	if req.Capabilities.Behavior == toolpolicy.BehaviorHumanOnly && req.Capabilities.Reason != "" {
		f.writeField(&b, "Policy", req.Capabilities.Reason, contentWidth)
	}
	if f.req.approval.Review != nil && !suppressReviewRationale(f.req.approval.Review.Rationale, f.req.approval.ReviewerFailure) {
		f.writeField(&b, "Review", f.req.approval.Review.Rationale, contentWidth)
	}
	if f.req.approval.ReviewerFailure != "" {
		f.writeField(&b, "Auto-review unavailable", f.req.approval.ReviewerFailure, contentWidth)
	}
	b.WriteString(mutedStyle.Render("Choose"))
	b.WriteString("\n")
	for i, option := range approvalOptions() {
		b.WriteString(f.renderAction(i, option.label, option.description))
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

func (f *approvalForm) writeField(b *strings.Builder, title, value string, width int) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	b.WriteString(mutedStyle.Render(title))
	b.WriteString("\n")
	wrapped := wrapText(value, max(width-2, 10))
	b.WriteString(indentLines(wrapped, "  "))
	b.WriteString("\n\n")
}

type approvalOption struct {
	label       string
	description string
}

func approvalOptions() []approvalOption {
	return []approvalOption{
		{label: "Yes", description: "Allow once"},
		{label: "Yes, session", description: "Remember for this session"},
		{label: "No", description: "Deny this request"},
		{label: "No + feedback", description: "Deny and tell the agent why"},
	}
}

func (f *approvalForm) renderAction(index int, label, description string) string {
	line := fmt.Sprintf("%-14s %s", label, mutedStyle.Render(description))
	if f.action == index {
		return askFocusedStyle.Render("❯ " + line)
	}
	return "  " + line
}

func approvalPrompt(req permission.Request) string {
	if req.ToolName == "bash" {
		return "Would you like to run the following command?"
	}
	return "Would you like to run the following tool call?"
}

func approvalActionTitle(req permission.Request) string {
	if req.ToolName == "bash" {
		return "Command"
	}
	return "Action"
}

func approvalActionValue(req permission.Request) string {
	action := strings.TrimSpace(req.Action)
	if req.ToolName == "bash" {
		action = strings.TrimPrefix(action, "bash ")
		return "$ " + strings.TrimSpace(action)
	}
	return action
}

func suppressReviewRationale(rationale, reviewerFailure string) bool {
	if strings.TrimSpace(reviewerFailure) == "" {
		return false
	}
	return strings.Contains(strings.ToLower(rationale), "auto-review was unavailable")
}
