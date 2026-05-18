package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const maxQueuedNoticeItems = 3

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
			Padding(1, 2)

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

	sidebarBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("27")).
			Padding(0, 1)
	sidebarTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("33"))
)

func (m Model) View() string {
	width := max(m.width, minWidth)
	height := max(m.height, minHeight)

	header := m.renderHeader(width)
	footer := m.renderFooter(width)
	contentWidth := width
	if shouldRenderSidebar(width) {
		contentWidth = width - sidebarWidth - sidebarGap
	}

	notice := m.renderRunningNotice(contentWidth)
	suggestions := m.renderSuggestions(contentWidth)
	input := m.renderInput(contentWidth)
	bodyHeight := height - lipgloss.Height(notice) - lipgloss.Height(suggestions) - lipgloss.Height(input) - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	body := m.renderBody(contentWidth, bodyHeight)

	mainParts := []string{body}
	if notice != "" {
		mainParts = append(mainParts, notice)
	}
	mainParts = append(mainParts, input)
	if suggestions != "" {
		mainParts = append(mainParts, suggestions)
	}
	content := lipgloss.JoinVertical(lipgloss.Left, mainParts...)
	if shouldRenderSidebar(width) {
		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			content,
			strings.Repeat(" ", sidebarGap),
			m.renderSidebar(sidebarWidth, lipgloss.Height(content)),
		)
	}

	parts := []string{content}
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

func (m Model) renderSidebar(width int, height int) string {
	docs := m.sidebarDocuments
	if len(docs) == 0 {
		docs = loadSidebarDocuments(m.runner.WorkDir())
	}
	if len(docs) == 0 {
		return ""
	}

	boxHeight := max(height/len(docs), 3)
	remainder := max(height-(boxHeight*len(docs)), 0)
	boxes := make([]string, 0, len(docs))
	for i, doc := range docs {
		currentHeight := boxHeight
		if i < remainder {
			currentHeight++
		}
		boxes = append(boxes, renderSidebarBox(doc, width, currentHeight))
	}
	return lipgloss.JoinVertical(lipgloss.Left, boxes...)
}

func renderSidebarBox(doc sidebarDocument, width int, height int) string {
	contentWidth := max(width-sidebarBoxStyle.GetHorizontalFrameSize(), 10)
	contentHeight := max(height-sidebarBoxStyle.GetVerticalFrameSize(), 1)
	bodyWidth := contentWidth

	title := sidebarTitleStyle.Render(doc.Title)
	text := doc.Content
	if doc.Error != "" {
		text = doc.Content + "\n" + doc.Error
	}
	rendered := renderMarkdown(text, bodyWidth)
	lines := strings.Split(rendered, "\n")
	availableBodyLines := max(contentHeight-1, 0)
	if len(lines) > availableBodyLines {
		if availableBodyLines <= 0 {
			lines = nil
		} else {
			lines = append(lines[:availableBodyLines-1], mutedStyle.Render("..."))
		}
	}
	for len(lines) < availableBodyLines {
		lines = append(lines, "")
	}

	contentLines := append([]string{fitLine(title, bodyWidth)}, lines...)
	content := strings.Join(contentLines, "\n")
	return sidebarBoxStyle.
		Width(contentWidth).
		Height(contentHeight).
		Render(content)
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
	meta := mutedStyle.Render(title)
	bodyWidth := max(width-2, 20)
	body := indentLines(renderMarkdown(e.body, bodyWidth), "  ")
	return fitLine(label+" "+meta, width) + "\n" + body
}

func renderUserEntry(e entry, width int) string {
	body := wrapText(e.body, max(width-userBubbleStyle.GetHorizontalFrameSize(), 20))
	return userBubbleStyle.Width(width - userBubbleStyle.GetHorizontalFrameSize()).Render(body)
}

func renderAssistantEntry(e entry, width int) string {
	meta := assistantLabelStyle.Render("Foxharness")
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
	if value == "" {
		placeholder := "Message foxharness, or type /help"
		if m.running {
			placeholder = "Message will be queued, or type /cancel"
		}
		value = renderCursor() + " " + placeholderStyle.Render(placeholder)
	} else {
		value += renderCursor()
	}
	return inputStyle.Width(width - inputStyle.GetHorizontalFrameSize()).Render(prompt + value)
}

func (m Model) renderRunningNotice(width int) string {
	if !m.running {
		return ""
	}
	queue := ""
	if len(m.queuedPrompts) > 0 {
		queue = fmt.Sprintf(" • %d queued", len(m.queuedPrompts))
	}
	lines := []string{
		fmt.Sprintf("%s Working (%s%s • esc to interrupt)", m.workingFrame(), formatDuration(m.runningElapsed()), queue),
	}
	lines = append(lines, queuedPromptNoticeLines(m.queuedPrompts, width)...)
	return runningNoticeStyle.Width(width - runningNoticeStyle.GetHorizontalFrameSize()).Render(strings.Join(lines, "\n"))
}

func queuedPromptNoticeLines(prompts []string, width int) []string {
	if len(prompts) == 0 {
		return nil
	}
	lineWidth := max(width-runningNoticeStyle.GetHorizontalFrameSize()-2, 20)
	count := min(len(prompts), maxQueuedNoticeItems)
	lines := make([]string, 0, count+1)
	for i := 0; i < count; i++ {
		prefix := fmt.Sprintf("  %d. ", i+1)
		messageWidth := max(lineWidth-lipgloss.Width(prefix), 1)
		message := strings.Join(strings.Fields(prompts[i]), " ")
		lines = append(lines, prefix+xansi.Truncate(message, messageWidth, "..."))
	}
	if remaining := len(prompts) - count; remaining > 0 {
		lines = append(lines, fmt.Sprintf("  ... %d more", remaining))
	}
	return lines
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

func (m Model) renderSuggestions(width int) string {
	if suggestions := m.renderSlashSuggestions(width); suggestions != "" {
		return suggestions
	}
	return m.renderFileMentionSuggestions(width)
}

func (m Model) renderFileMentionSuggestions(width int) string {
	matches := m.matchingFileMentions()
	if len(matches) == 0 {
		return ""
	}

	menuWidth := max(width-suggestionStyle.GetHorizontalFrameSize(), 20)
	rowWidth := max(menuWidth-suggestionStyle.GetHorizontalFrameSize(), 20)
	lines := make([]string, 0, len(matches))
	selected := m.fileSelection
	if selected < 0 || selected >= len(matches) {
		selected = 0
	}
	for i, mention := range matches {
		line := fileMentionSuggestionPlainLine(mention, "  ")
		if i == selected {
			line = fileMentionSuggestionPlainLine(mention, "❯ ")
			line = xansi.Truncate(line, rowWidth, "...")
			lines = append(lines, suggestionSelectedStyle.Width(rowWidth).Render(line))
			continue
		}
		line = xansi.Truncate(line, rowWidth, "...")
		lines = append(lines, suggestionCommandStyle.Render(line))
	}
	return suggestionStyle.Width(menuWidth).Render(strings.Join(lines, "\n"))
}

func fileMentionSuggestionPlainLine(mention fileMention, pointer string) string {
	return pointer + "@" + mention.Path
}

func renderCursor() string {
	return cursorStyle.Render(" ")
}

func (m Model) renderFooter(width int) string {
	help := "Enter send | Up/Down history | Tab complete | Shift+Tab plan | PgUp/PgDown/wheel scroll | Ctrl+C twice quit"
	if m.hasSlashMenu() {
		help = "Up/Down select | Tab complete | Enter run | Esc close | Ctrl+C twice quit"
	} else if m.hasFileMentionMenu() {
		help = "Up/Down select | Tab complete file | Enter send | Esc close | Ctrl+C twice quit"
	} else if m.running {
		help = "Enter queue | Shift+Tab toggles next run | Esc cancel current run | Ctrl+C twice quit"
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
