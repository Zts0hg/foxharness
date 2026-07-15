package tui

import (
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/permission"
	tea "github.com/charmbracelet/bubbletea"
)

type permissionDoneMsg struct{}

type permissionFormStage int

const (
	permissionFormStageModes permissionFormStage = iota
	permissionFormStageFullAccessWarning
)

type permissionFormAction int

const (
	permissionFormCancel permissionFormAction = iota
	permissionFormAsk
	permissionFormApprove
	permissionFormFullAccessSession
	permissionFormFullAccessRemember
	permissionFormClear
)

type permissionForm struct {
	stage    permissionFormStage
	cursor   int
	snapshot permission.Snapshot
	result   permissionFormAction
	done     bool
}

func newPermissionForm(snapshot permission.Snapshot) *permissionForm {
	f := &permissionForm{snapshot: snapshot}
	switch snapshot.SelectedMode {
	case permission.ModeApprove:
		f.cursor = 1
	case permission.ModeFullAccess:
		f.cursor = 2
	default:
		f.cursor = 0
	}
	return f
}

func newFullAccessWarningForm(snapshot permission.Snapshot) *permissionForm {
	return &permissionForm{stage: permissionFormStageFullAccessWarning, snapshot: snapshot}
}

func (f *permissionForm) update(msg tea.KeyMsg) tea.Cmd {
	if f.done {
		return nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		f.result = permissionFormCancel
		return f.submit()
	case tea.KeyTab, tea.KeyDown, tea.KeyRight:
		f.move(1)
	case tea.KeyShiftTab, tea.KeyUp, tea.KeyLeft:
		f.move(-1)
	case tea.KeyRunes:
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'j':
				f.move(1)
			case 'k':
				f.move(-1)
			case '1', '2', '3', '4':
				index := int(msg.Runes[0] - '1')
				if index < f.optionCount() {
					f.cursor = index
					return f.selectCurrent()
				}
			}
		}
	case tea.KeyEnter:
		return f.selectCurrent()
	}
	return nil
}

func (f *permissionForm) move(delta int) {
	count := f.optionCount()
	if count == 0 {
		return
	}
	f.cursor = (f.cursor + delta + count) % count
}

func (f *permissionForm) optionCount() int {
	if f.stage == permissionFormStageFullAccessWarning {
		return 3
	}
	return 4
}

func (f *permissionForm) selectCurrent() tea.Cmd {
	if f.stage == permissionFormStageFullAccessWarning {
		switch f.cursor {
		case 0:
			f.result = permissionFormFullAccessSession
		case 1:
			f.result = permissionFormFullAccessRemember
		default:
			f.result = permissionFormCancel
		}
		return f.submit()
	}
	switch f.cursor {
	case 0:
		f.result = permissionFormAsk
	case 1:
		f.result = permissionFormApprove
	case 2:
		f.stage = permissionFormStageFullAccessWarning
		f.cursor = 0
		return nil
	default:
		f.result = permissionFormClear
	}
	return f.submit()
}

func (f *permissionForm) submit() tea.Cmd {
	f.done = true
	return func() tea.Msg { return permissionDoneMsg{} }
}

func (f *permissionForm) view(width int) string {
	var b strings.Builder
	if f.stage == permissionFormStageFullAccessWarning {
		b.WriteString(headerStyle.Render("Full Access warning"))
		b.WriteString("\n\n")
		b.WriteString("Full Access runs tool calls without approval prompts in this session.\n")
		b.WriteString("Use it only when you trust the workspace, commands, and requested changes.\n\n")
		for i, label := range []string{"Confirm for this session", "Confirm and remember", "Cancel"} {
			b.WriteString(f.renderAction(i, label))
			b.WriteString("\n")
		}
		b.WriteString("\n\n")
		b.WriteString(hintStyle.Render("Tab/↑/↓ choose · Enter confirm · Esc cancel"))
		return f.fit(width, b.String())
	}

	b.WriteString(headerStyle.Render("Permissions"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Selected: %s\n", permissionModeLabel(f.snapshot.SelectedMode)))
	b.WriteString(fmt.Sprintf("Effective: %s\n", permissionModeLabel(f.snapshot.EffectiveMode)))
	b.WriteString(fmt.Sprintf("Session approvals: %d\n", f.snapshot.SessionGrantCount))
	b.WriteString(fmt.Sprintf("Full Access warning remembered: %s\n\n", onOff(f.snapshot.FullAccessRemembered)))

	options := []struct {
		label string
		desc  string
	}{
		{"Ask for approval", "Prompt before reviewed tool calls."},
		{"Approve for me", "Auto-review tool calls; ask when the reviewer escalates or is unavailable."},
		{"Full Access", "Switch after an explicit warning confirmation."},
		{"Clear session approvals", "Remove exact-invocation approvals granted in this session."},
	}
	for i, option := range options {
		b.WriteString(f.renderAction(i, option.label))
		b.WriteString(mutedStyle.Render("  " + option.desc))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Tab/↑/↓ choose · Enter confirm · Esc cancel"))
	return f.fit(width, b.String())
}

func (f *permissionForm) fit(width int, content string) string {
	contentWidth := max(width-inputStyle.GetHorizontalFrameSize()-2, 20)
	lines := strings.Split(content, "\n")
	for i := range lines {
		lines[i] = fitLine(lines[i], contentWidth)
	}
	return strings.Join(lines, "\n")
}

func (f *permissionForm) renderAction(index int, label string) string {
	if f.cursor == index {
		return askFocusedStyle.Render("❯ " + label)
	}
	return "  " + label
}
