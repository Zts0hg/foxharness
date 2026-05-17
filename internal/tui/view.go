package tui

import (
	"fmt"
	"strings"
	"time"

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

	suggestionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(lipgloss.Color("235")).
			Padding(0, 1)

	suggestionCommandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("81"))

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	userBubbleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("24")).
			Padding(0, 1)

	userMetaStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("195"))
	assistantLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("114"))
	toolLabelStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	systemLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("147"))
	errorLabelStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	placeholderStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cursorStyle         = lipgloss.NewStyle().Reverse(true)
	planModeStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
)

func (m Model) View() string {
	width := max(m.width, minWidth)
	height := max(m.height, minHeight)

	header := m.renderHeader(width)
	notice := m.renderRunningNotice(width)
	suggestions := m.renderSlashSuggestions(width)
	input := m.renderInput(width)
	footer := m.renderFooter(width)
	bodyHeight := height - lipgloss.Height(header) - lipgloss.Height(notice) - lipgloss.Height(suggestions) - lipgloss.Height(input) - lipgloss.Height(footer)
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	body := m.renderBody(width, bodyHeight)

	parts := []string{header, body}
	if notice != "" {
		parts = append(parts, notice)
	}
	if suggestions != "" {
		parts = append(parts, suggestions)
	}
	parts = append(parts, input, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderHeader(width int) string {
	title := headerStyle.Render("FOXHARNESS")
	meta := fmt.Sprintf(" model %s  %s session %s", shortValue(m.modelName, 18), planModeText(m.planMode), shortValue(m.sessionID, 24))
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
	var out strings.Builder
	for i, e := range m.entries {
		if i > 0 {
			if isToolResultPair(m.entries[i-1], e) {
				out.WriteString("\n")
			} else {
				out.WriteString("\n\n")
			}
		}
		out.WriteString(renderEntry(e, width))
	}
	return out.String()
}

func renderEntry(e entry, width int) string {
	switch {
	case e.role == "user":
		return renderUserEntry(e, width)
	case e.role == "tool" && strings.HasPrefix(e.title, "call "):
		return renderToolCall(e, width)
	case e.role == "tool" && strings.HasPrefix(e.title, "result "):
		return renderToolResult(e, width)
	case e.role == "assistant":
		return renderAssistantEntry(e, width)
	}

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

func renderUserEntry(e entry, width int) string {
	meta := userMetaStyle.Render("You " + e.time.Format("15:04:05"))
	body := wrapText(e.body, max(width-userBubbleStyle.GetHorizontalFrameSize(), 20))
	content := meta
	if body != "" {
		content += "\n" + body
	}
	return userBubbleStyle.Width(width - userBubbleStyle.GetHorizontalFrameSize()).Render(content)
}

func renderAssistantEntry(e entry, width int) string {
	meta := assistantLabelStyle.Render("Foxharness") + " " + mutedStyle.Render(e.time.Format("15:04:05"))
	body := indentLines(wrapText(e.body, max(width-2, 20)), "  ")
	return fitLine(meta, width) + "\n" + body
}

func renderToolCall(e entry, width int) string {
	line := toolLabelStyle.Render("• " + strings.TrimSpace(e.body))
	return fitLine(line, width)
}

func renderToolResult(e entry, width int) string {
	output := strings.TrimSpace(e.body)
	if output == "" {
		output = "(no output)"
	}
	lines := strings.Split(wrapText(output, max(width-4, 20)), "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = "  └ " + lines[i]
		} else {
			lines[i] = "    " + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func isToolResultPair(prev entry, current entry) bool {
	return prev.role == "tool" &&
		current.role == "tool" &&
		strings.HasPrefix(prev.title, "call ") &&
		strings.HasPrefix(current.title, "result ")
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
	text := fmt.Sprintf("%s Working (%s • esc to interrupt)", m.workingFrame(), formatDuration(m.runningElapsed()))
	return runningNoticeStyle.Width(width - runningNoticeStyle.GetHorizontalFrameSize()).Render(text)
}

func (m Model) renderSlashSuggestions(width int) string {
	matches := m.matchingSlashCommands()
	if len(matches) == 0 {
		return ""
	}

	var parts []string
	for _, command := range matches {
		parts = append(parts, fmt.Sprintf("%s %s", suggestionCommandStyle.Render(command.Name), command.Description))
	}
	text := "Tab complete  " + strings.Join(parts, "   ")
	return suggestionStyle.Width(width - suggestionStyle.GetHorizontalFrameSize()).Render(fitLine(text, width-suggestionStyle.GetHorizontalFrameSize()))
}

func renderCursor() string {
	return cursorStyle.Render(" ")
}

func (m Model) renderFooter(width int) string {
	help := "Enter send | Up/Down history | Tab complete | Shift+Tab plan | PgUp/PgDown scroll | Ctrl+C twice quit"
	if m.running {
		help = "Shift+Tab toggles plan for next run | Esc cancel current run | Ctrl+C twice quit"
	}
	line := fmt.Sprintf("%s  %s", m.status, help)
	return footerStyle.Width(width).Render(fitLine(line, width))
}

func planModeText(enabled bool) string {
	if !enabled {
		return ""
	}
	return planModeStyle.Render("plan mode on")
}

func labelStyle(e entry) lipgloss.Style {
	if e.err {
		return errorLabelStyle
	}
	switch e.role {
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

func formatDuration(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	if total < 0 {
		total = 0
	}
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	minutes := total / 60
	seconds := total % 60
	return fmt.Sprintf("%dm%02ds", minutes, seconds)
}

func shortValue(s string, limit int) string {
	if len([]rune(s)) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit-3]) + "..."
}
