package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type effortDoneMsg struct{}

type effortForm struct {
	protocol string
	options  []string
	cursor   int
	result   string
	done     bool
}

func newEffortForm(protocol string, options []string, selected string) *effortForm {
	f := &effortForm{protocol: protocol, options: append([]string(nil), options...)}
	for i, option := range f.options {
		if option == selected {
			f.cursor = i
			break
		}
	}
	return f
}

func (f *effortForm) update(msg tea.KeyMsg) tea.Cmd {
	if f.done {
		return nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		f.done = true
		return func() tea.Msg { return effortDoneMsg{} }
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
			case '1', '2', '3', '4', '5', '6', '7':
				index := int(msg.Runes[0] - '1')
				if index < len(f.options) {
					f.cursor = index
					return f.submit()
				}
			}
		}
	case tea.KeyEnter:
		return f.submit()
	}
	return nil
}

func (f *effortForm) move(delta int) {
	if len(f.options) == 0 {
		return
	}
	f.cursor = (f.cursor + delta + len(f.options)) % len(f.options)
}

func (f *effortForm) submit() tea.Cmd {
	if len(f.options) > 0 {
		f.result = f.options[f.cursor]
	}
	f.done = true
	return func() tea.Msg { return effortDoneMsg{} }
}

func (f *effortForm) view(width int) string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Effort"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("Protocol: %s\n\n", f.protocol))
	for i, option := range f.options {
		if f.cursor == i {
			b.WriteString(askFocusedStyle.Render("❯ " + option))
		} else {
			b.WriteString("  " + option)
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(hintStyle.Render("Tab/↑/↓ choose · Enter confirm · Esc cancel"))
	contentWidth := max(width-inputStyle.GetHorizontalFrameSize()-2, 20)
	lines := strings.Split(b.String(), "\n")
	for i := range lines {
		lines[i] = fitLine(lines[i], contentWidth)
	}
	return strings.Join(lines, "\n")
}
