package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("30")).
			Padding(0, 1)

	headerMetaStyle = lipgloss.NewStyle().
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
			Padding(0, 1)

	suggestionCommandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("252"))
	suggestionSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("81"))
	suggestionDescriptionStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("252"))

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
	commandLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("147"))
	mutedStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	placeholderStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	cursorStyle         = lipgloss.NewStyle().Reverse(true)
	planModeStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	statusModelStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	statusProjectStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	statusGitStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	statusDimStyle      = lipgloss.NewStyle().Faint(true)
	contextLowStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	contextMediumStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	contextHighStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

func (m Model) View() string {
	width := max(m.width, minWidth)
	height := max(m.height, minHeight)

	header := m.renderHeader(width)
	notice := m.renderRunningNotice(width)
	suggestions := m.renderSlashSuggestions(width)
	input := m.renderInput(width)
	footer := m.renderFooter(width)
	bodyHeight := height - lipgloss.Height(notice) - lipgloss.Height(suggestions) - lipgloss.Height(input) - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	body := m.renderBody(width, bodyHeight)

	parts := []string{body}
	if notice != "" {
		parts = append(parts, notice)
	}
	parts = append(parts, input)
	if suggestions != "" {
		parts = append(parts, suggestions)
	}
	parts = append(parts, header, footer)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) renderHeader(width int) string {
	title := headerStyle.Render("FOXHARNESS")
	meta := m.renderHeaderMeta()
	metaWidth := max(width-lipgloss.Width(title)-headerMetaStyle.GetHorizontalFrameSize(), 1)
	meta = xansi.Truncate(meta, metaWidth, "...")
	right := headerMetaStyle.Width(max(width-lipgloss.Width(title), 1)).Render(meta)
	header := lipgloss.JoinHorizontal(lipgloss.Top, title, right)
	if !m.planMode {
		return header
	}
	return lipgloss.JoinVertical(lipgloss.Left, header, headerMetaStyle.Width(width-headerMetaStyle.GetHorizontalFrameSize()).Render(planModeText(true)))
}

func (m Model) renderHeaderMeta() string {
	sep := " " + statusDimStyle.Render("|") + " "
	return statusModelStyle.Render("["+m.modelName+"]") +
		" " + statusProjectStyle.Render(m.project) +
		sep + statusGitStyle.Render("git:("+m.gitBranch+")") +
		sep + "Context: " + renderContextUsage(m.contextUsage) +
		sep + statusDimStyle.Render("sid:"+m.sessionID)
}

func renderContextUsage(usage string) string {
	percent, ok, display := parseContextUsage(usage)
	if !ok {
		return statusDimStyle.Render(normalizeContextUsage(usage))
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := percent / 10
	bar := strings.Repeat("▓", filled) + strings.Repeat("░", 10-filled)
	value := bar + " " + display
	return contextUsageStyle(percent).Render(value)
}

func parseContextUsage(usage string) (int, bool, string) {
	usage = strings.TrimSpace(usage)
	if usage == "" || usage == "unknown" {
		return 0, false, ""
	}
	if strings.HasPrefix(usage, "<") {
		return 0, true, usage
	}
	if strings.HasSuffix(usage, "%") {
		raw := strings.TrimSpace(strings.TrimSuffix(usage, "%"))
		var percent int
		if _, err := fmt.Sscanf(raw, "%d", &percent); err == nil {
			return percent, true, fmt.Sprintf("%d%%", percent)
		}
	}
	return 0, false, usage
}

func contextUsageStyle(percent int) lipgloss.Style {
	if percent >= 75 {
		return contextHighStyle
	}
	if percent >= 50 {
		return contextMediumStyle
	}
	return contextLowStyle
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
	case e.role == "command":
		return renderCommandEntry(e, width)
	}

	label := labelStyle(e).Render(strings.ToUpper(e.role))
	title := strings.TrimSpace(e.title)
	if title == "" {
		title = e.role
	}
	meta := mutedStyle.Render(fmt.Sprintf("%s  %s", title, e.time.Format("15:04:05")))
	bodyWidth := max(width-2, 20)
	body := indentLines(renderMarkdown(e.body, bodyWidth), "  ")
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
	body := indentLines(renderMarkdown(e.body, max(width-2, 20)), "  ")
	return fitLine(meta, width) + "\n" + body
}

func renderCommandEntry(e entry, width int) string {
	title := strings.TrimSpace(e.title)
	if title == "" {
		title = "Result"
	}
	header := fitLine(commandLabelStyle.Render(title), width)
	body := renderPlainBlock(e.body, max(width-2, 20))
	if body == "" {
		return header
	}
	return header + "\n" + indentLines(body, "  ")
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

	menuWidth := max(width-suggestionStyle.GetHorizontalFrameSize(), 20)
	rowWidth := max(menuWidth-suggestionStyle.GetHorizontalFrameSize(), 20)
	lines := make([]string, 0, len(matches))
	selected := m.slashSelection
	if selected < 0 || selected >= len(matches) {
		selected = 0
	}
	for i, command := range matches {
		if i == selected {
			line := slashSuggestionPlainLine(command, "❯ ")
			line = xansi.Truncate(line, rowWidth, "...")
			lines = append(lines, suggestionSelectedStyle.Width(rowWidth).Render(line))
			continue
		}
		line := "  " +
			suggestionCommandStyle.Render(command.Name) +
			strings.Repeat(" ", max(14-lipgloss.Width(command.Name), 2)) +
			suggestionDescriptionStyle.Render(command.Description)
		line = xansi.Truncate(line, rowWidth, "...")
		lines = append(lines, line)
	}
	return suggestionStyle.Width(menuWidth).Render(strings.Join(lines, "\n"))
}

func slashSuggestionPlainLine(command slashCommand, pointer string) string {
	return pointer +
		command.Name +
		strings.Repeat(" ", max(14-lipgloss.Width(command.Name), 2)) +
		command.Description
}

func renderCursor() string {
	return cursorStyle.Render(" ")
}

func (m Model) renderFooter(width int) string {
	help := "Enter send | Up/Down history | Tab complete | Shift+Tab plan | PgUp/PgDown scroll | Ctrl+C twice quit"
	if m.running {
		help = "Shift+Tab toggles plan for next run | Esc cancel current run | Ctrl+C twice quit"
	} else if m.hasSlashMenu() {
		help = "Up/Down select | Tab complete | Enter run | Esc close | Ctrl+C twice quit"
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

func renderPlainBlock(text string, width int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		out = append(out, wrapPlainLine(line, width)...)
	}
	return strings.Join(out, "\n")
}

func wrapPlainLine(line string, width int) []string {
	line = strings.TrimRight(line, " \t")
	if line == "" {
		return []string{""}
	}
	if lipgloss.Width(line) <= width {
		return []string{line}
	}

	continuation := continuationIndent(line)
	var lines []string
	current := line
	for lipgloss.Width(current) > width {
		head, tail := splitLineAtWidth(current, width)
		if strings.TrimSpace(head) == "" {
			break
		}
		lines = append(lines, head)
		if strings.TrimSpace(tail) == "" {
			return lines
		}
		current = continuation + strings.TrimSpace(tail)
	}
	lines = append(lines, current)
	return lines
}

func splitLineAtWidth(line string, width int) (string, string) {
	runes := []rune(line)
	lastSpace := -1
	for i := range runes {
		candidate := string(runes[:i+1])
		if lipgloss.Width(candidate) > width {
			if lastSpace > 0 {
				return strings.TrimRight(string(runes[:lastSpace]), " "), strings.TrimLeft(string(runes[lastSpace+1:]), " ")
			}
			return strings.TrimRight(string(runes[:i]), " "), strings.TrimLeft(string(runes[i:]), " ")
		}
		if runes[i] == ' ' || runes[i] == '\t' {
			lastSpace = i
		}
	}
	return line, ""
}

func continuationIndent(line string) string {
	index := strings.Index(line, "  ")
	if index < 0 {
		return "  "
	}
	return strings.Repeat(" ", min(index+2, 16))
}
