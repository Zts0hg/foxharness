package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("30")).
			Padding(0, 1)

	headerMetaStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("253")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	bodyStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("36")).
			Padding(0, 1)

	runningNoticeStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("94")).
				Padding(0, 1)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	userLabelStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
	assistantLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("114"))
	toolLabelStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	systemLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("147"))
	errorLabelStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	placeholderStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cursorStyle         = lipgloss.NewStyle().Reverse(true)
)

func (m Model) View() string {
	width := max(m.width, minWidth)
	height := max(m.height, minHeight)

	header := m.renderHeader(width)
	notice := m.renderRunningNotice(width)
	input := m.renderInput(width)
	footer := m.renderFooter(width)
	bodyHeight := height - lipgloss.Height(header) - lipgloss.Height(notice) - lipgloss.Height(input) - lipgloss.Height(footer)
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	body := m.renderBody(width, bodyHeight)

	parts := []string{header, body}
	if notice != "" {
		parts = append(parts, notice)
	}
	parts = append(parts, input, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderHeader(width int) string {
	title := headerStyle.Render("FOXHARNESS")
	meta := fmt.Sprintf(" model %s  session %s", shortValue(m.modelName, 18), shortValue(m.sessionID, 24))
	right := headerMetaStyle.Width(max(width-lipgloss.Width(title), 1)).Render(meta)
	return lipgloss.JoinHorizontal(lipgloss.Top, title, right)
}

func (m Model) renderBody(width int, height int) string {
	contentWidth := max(width-bodyStyle.GetHorizontalFrameSize()-2, 20)
	content := m.renderEntries(contentWidth)
	lines := strings.Split(content, "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{placeholderStyle.Render("Start typing below. Use /help for commands.")}
	}

	visible := max(height-bodyStyle.GetVerticalFrameSize(), 1)
	start := len(lines) - visible - m.scrollOffset
	if start < 0 {
		start = 0
	}
	end := min(start+visible, len(lines))
	view := strings.Join(lines[start:end], "\n")
	return bodyStyle.Width(width - bodyStyle.GetHorizontalFrameSize()).Height(height - bodyStyle.GetVerticalFrameSize()).Render(view)
}

func (m Model) renderEntries(width int) string {
	var parts []string
	for _, e := range m.entries {
		parts = append(parts, renderEntry(e, width))
	}
	return strings.Join(parts, "\n\n")
}

func renderEntry(e entry, width int) string {
	label := labelStyle(e).Render(strings.ToUpper(e.role))
	title := strings.TrimSpace(e.title)
	if title == "" {
		title = e.role
	}
	meta := mutedStyle.Render(fmt.Sprintf("%s  %s", title, e.time.Format("15:04:05")))
	bodyWidth := max(width-2, 20)
	body := indentLines(wrapText(e.body, bodyWidth), "  ")
	return fitLine(label+" "+meta, width) + "\n" + body
}

func (m Model) renderInput(width int) string {
	prompt := "> "
	value := string(m.input)
	if m.running {
		value = mutedStyle.Render("Input locked until the current run completes.")
	} else if value == "" {
		value = renderCursor() + " " + placeholderStyle.Render("Message foxharness, or type /help")
	} else {
		value += renderCursor()
	}
	return inputStyle.Width(width - inputStyle.GetHorizontalFrameSize()).Render(prompt + value)
}

func (m Model) renderRunningNotice(width int) string {
	if !m.running {
		return ""
	}
	text := "Agent is running. Press Esc to request cancellation."
	return runningNoticeStyle.Width(width - runningNoticeStyle.GetHorizontalFrameSize()).Render(text)
}

func renderCursor() string {
	return cursorStyle.Render(" ")
}

func (m Model) renderFooter(width int) string {
	help := "Enter send | Up/PgUp scroll | /session | /new | /clear | Ctrl+C quit"
	if m.running {
		help = "Esc cancel current run | Ctrl+C quit"
	}
	line := fmt.Sprintf("%s  %s", m.status, help)
	return footerStyle.Width(width).Render(fitLine(line, width))
}

func labelStyle(e entry) lipgloss.Style {
	if e.err {
		return errorLabelStyle
	}
	switch e.role {
	case "user":
		return userLabelStyle
	case "assistant":
		return assistantLabelStyle
	case "tool":
		return toolLabelStyle
	case "error":
		return errorLabelStyle
	default:
		return systemLabelStyle
	}
}

func wrapText(text string, width int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var out []string
	for _, paragraph := range strings.Split(text, "\n") {
		out = append(out, wrapParagraph(paragraph, width)...)
	}
	return strings.Join(out, "\n")
}

func wrapParagraph(text string, width int) []string {
	if strings.TrimSpace(text) == "" {
		return []string{""}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			lines = appendWord(lines, &current, word, width)
			continue
		}
		next := current + " " + word
		if lipgloss.Width(next) <= width {
			current = next
			continue
		}
		lines = append(lines, current)
		current = ""
		lines = appendWord(lines, &current, word, width)
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func appendWord(lines []string, current *string, word string, width int) []string {
	if lipgloss.Width(word) <= width {
		*current = word
		return lines
	}
	var chunk []rune
	for _, r := range word {
		chunk = append(chunk, r)
		if lipgloss.Width(string(chunk)) >= width {
			lines = append(lines, string(chunk))
			chunk = nil
		}
	}
	if len(chunk) > 0 {
		*current = string(chunk)
	}
	return lines
}

func indentLines(text string, prefix string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func fitLine(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+3 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "..."
}

func shortValue(s string, limit int) string {
	if len([]rune(s)) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-3]) + "..."
}
