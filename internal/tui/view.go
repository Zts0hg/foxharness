package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const (
	maxQueuedNoticeItems = 3

	viewPaddingTop    = 1
	viewPaddingRight  = 2
	viewPaddingBottom = 0
	viewPaddingLeft   = 2

	amberBgHex            = "#0a0703"
	amberPanelHex         = "#171006"
	amberHex              = "#ffc56b"
	amberHiHex            = "#ffe3a8"
	amberWarnHex          = "#ff8855"
	amberMutedHex         = "#D69D60"
	amberDimHex           = "#B07E48"
	amberDividerHex       = "#6b4520"
	amberProgressEmptyHex = "#5a3020"
)

var (
	cBg            = lipgloss.Color(amberBgHex)
	cAccent        = lipgloss.Color(amberHex)
	cAccentHi      = lipgloss.Color(amberHiHex)
	cWarn          = lipgloss.Color(amberWarnHex)
	cTextPri       = lipgloss.Color(amberHiHex)
	cTextSec       = lipgloss.Color(amberHex)
	cTextMuted     = lipgloss.Color(amberMutedHex)
	cTextDim       = lipgloss.Color(amberDimHex)
	cTextVeryDim   = lipgloss.Color(amberDividerHex)
	cMsgBg         = lipgloss.Color(amberPanelHex)
	cProgressEmpty = lipgloss.Color(amberProgressEmptyHex)

	outerStyle = lipgloss.NewStyle().
			Foreground(cAccent).
			Padding(viewPaddingTop, viewPaddingRight, viewPaddingBottom, viewPaddingLeft)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(cAccent)

	headerMetaStyle = lipgloss.NewStyle().
			Foreground(cTextDim)

	bodyStyle = lipgloss.NewStyle().
			Foreground(cTextSec)

	inputStyle = lipgloss.NewStyle().
			Foreground(cTextSec).
			Border(lipgloss.Border{Top: "─", Bottom: "─"}, true, false, true, false).
			BorderForeground(cTextVeryDim)

	runningNoticeStyle = lipgloss.NewStyle().
				Foreground(cWarn)

	suggestionStyle = lipgloss.NewStyle().
			Foreground(cTextSec).
			Border(lipgloss.Border{Left: "┊"}, false, false, false, true).
			BorderForeground(cTextVeryDim).
			Padding(0, 1)

	suggestionCommandStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(cAccentHi)
	suggestionSelectedStyle = lipgloss.NewStyle().
				Foreground(cWarn)
	suggestionDescriptionStyle = lipgloss.NewStyle().
					Foreground(cTextMuted)

	footerStyle = lipgloss.NewStyle().
			Foreground(cTextMuted)

	userBubbleStyle = lipgloss.NewStyle().
			Foreground(cAccent).
			Background(cMsgBg).
			Border(lipgloss.Border{Left: "▌"}, false, false, false, true).
			BorderForeground(cAccent).
			BorderBackground(cMsgBg).
			Padding(0, 1)

	userMetaStyle       = lipgloss.NewStyle().Bold(true).Foreground(cAccentHi)
	assistantLabelStyle = lipgloss.NewStyle().Foreground(cAccentHi)
	toolLabelStyle      = lipgloss.NewStyle().Foreground(cWarn)
	systemLabelStyle    = lipgloss.NewStyle().Bold(true).Foreground(cTextSec)
	errorLabelStyle     = lipgloss.NewStyle().Bold(true).Foreground(cWarn)
	commandLabelStyle   = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	mutedStyle          = lipgloss.NewStyle().Foreground(cTextMuted)
	placeholderStyle    = lipgloss.NewStyle().Foreground(cTextDim).Italic(true)
	cursorStyle         = lipgloss.NewStyle().Foreground(cAccentHi)
	hintStyle           = lipgloss.NewStyle().Foreground(cTextMuted)
	planModeStyle       = lipgloss.NewStyle().Foreground(cWarn)
	statusModelStyle    = lipgloss.NewStyle().Foreground(cAccentHi)
	statusProjectStyle  = lipgloss.NewStyle().Foreground(cTextMuted)
	statusGitStyle      = lipgloss.NewStyle().Foreground(cWarn)
	statusDimStyle      = lipgloss.NewStyle().Foreground(cTextVeryDim)
	statusFaintStyle    = lipgloss.NewStyle().Foreground(cTextDim)
	contextLowStyle     = lipgloss.NewStyle().Foreground(cAccentHi)
	contextMediumStyle  = lipgloss.NewStyle().Foreground(cWarn)
	contextHighStyle    = lipgloss.NewStyle().Foreground(cWarn)

	sidebarBoxStyle = lipgloss.NewStyle().
			Foreground(cTextMuted)
	sidebarFocusedBoxStyle = lipgloss.NewStyle().
				Foreground(cTextSec)
	sidebarTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(cAccent)
	sidebarFocusedTitleStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(cWarn)
	sidebarDividerStyle = lipgloss.NewStyle().Foreground(cTextVeryDim)
)

func (m Model) View() string {
	width := m.innerWidth()
	if m.width < minWidth {
		return lipgloss.NewStyle().Foreground(cWarn).Padding(2).Render(
			fmt.Sprintf("terminal too narrow (%d cols) - please widen to at least %d cols", m.width, minWidth))
	}
	if m.rewindSelector != nil {
		parts := []string{
			m.renderHeader(width),
			"",
			m.rewindSelector.View(),
			"",
			m.renderStatusBar(width),
			m.renderKeybinds(width),
		}
		return outerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	_, bodyHeight := m.contentDimensions()
	parts := []string{
		m.renderMainArea(bodyHeight),
	}
	notice := m.renderRunningNotice(width)
	if notice != "" {
		parts = append(parts, notice)
	}
	parts = append(parts, m.renderInput(width))
	if suggestions := m.renderSuggestions(width); suggestions != "" {
		parts = append(parts, suggestions)
	}
	parts = append(parts,
		"",
		m.renderStatusBar(width),
		m.renderKeybinds(width),
	)
	return outerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
}

func (m Model) contentDimensions() (int, int) {
	height := max(m.height, minHeight)
	contentWidth := m.chatWidth()
	if m.shouldRenderSidebar() {
		contentWidth = m.chatWidth()
	}

	innerWidth := m.innerWidth()
	chrome := outerStyle.GetVerticalFrameSize() +
		lipgloss.Height(m.renderInput(innerWidth)) +
		1 /* footer gap */ +
		lipgloss.Height(m.renderStatusBar(innerWidth)) +
		lipgloss.Height(m.renderKeybinds(innerWidth))
	if notice := m.renderRunningNotice(innerWidth); notice != "" {
		chrome += lipgloss.Height(notice)
	}
	if suggestions := m.renderSuggestions(innerWidth); suggestions != "" {
		chrome += lipgloss.Height(suggestions)
	}
	bodyHeight := height - chrome
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	return contentWidth, bodyHeight
}

func (m Model) innerWidth() int {
	width := max(m.width, minWidth)
	inner := width - outerStyle.GetHorizontalFrameSize()
	if inner < 10 {
		return 10
	}
	return inner
}

func (m Model) chatWidth() int {
	width := m.innerWidth()
	if m.shouldRenderSidebar() {
		width -= m.sidebarWidth() + sidebarGap
	}
	if width < 20 {
		return 20
	}
	return width
}

func (m Model) sidebarWidth() int {
	width := m.innerWidth() * 26 / 100
	if width < sidebarWidth {
		return sidebarWidth
	}
	if width > sidebarMaxWidth {
		return sidebarMaxWidth
	}
	return width
}

func (m Model) renderHeader(width int) string {
	name := headerStyle.Render("FOX-HARNESS")
	badge := statusModelStyle.Render("[ ESTABLISHED ]")
	subText := "// expert coding assistant // agent harness"
	nameW := lipgloss.Width(name)
	badgeW := lipgloss.Width(badge)
	if width >= nameW+badgeW+len(subText)+6 {
		sub := headerMetaStyle.Render(" " + subText + " ")
		dotsW := width - nameW - lipgloss.Width(sub) - badgeW - 2
		if dotsW < 1 {
			dotsW = 1
		}
		return name + sub + statusDimStyle.Render(" "+strings.Repeat("·", dotsW)+" ") + badge
	}
	dotsW := width - nameW - badgeW - 2
	if dotsW < 1 {
		dotsW = 1
	}
	return name + statusDimStyle.Render(" "+strings.Repeat("·", dotsW)+" ") + badge
}

func (m Model) renderSessionLine(width int) string {
	line := fmt.Sprintf("SESS#%s · STARTED %s · TZ %s", m.sessionID, m.nowTime().Format("15:04:05"), m.nowTime().Format("MST"))
	return headerMetaStyle.Width(width).Render(fitLine(line, width))
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
	contentWidth := max(width, 20)
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
	return bodyStyle.Width(width).Height(height).Render(view)
}

func (m Model) renderMainArea(height int) string {
	chat := m.renderBody(m.chatWidth(), height)
	if !m.shouldRenderSidebar() {
		return chat
	}
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		chat,
		strings.Repeat(" ", sidebarGap),
		m.renderSidebar(m.sidebarWidth(), height),
	)
}

func (m Model) renderSidebar(width int, height int) string {
	docs := m.sidebarDocuments
	if len(docs) == 0 {
		docs = loadSidebarDocuments(m.runner.WorkDir(), m.runner.SessionDir())
	}
	if len(docs) == 0 {
		return ""
	}

	contentWidth := sidebarContentWidth(width)
	boxesHeight := sidebarDocumentAreaHeight(height, len(docs))
	boxHeights := sidebarBoxHeights(boxesHeight, len(docs))
	sections := make([]string, 0, len(docs)+1)
	for i, doc := range docs {
		offset := 0
		if i < len(m.sidebarScrollOffsets) {
			offset = m.sidebarScrollOffsets[i]
		}
		focused := m.sidebarFocused && i == m.sidebarFocusIndex
		sections = append(sections, renderSidebarBoxWithFocus(doc, contentWidth, boxHeights[i], offset, focused))
		if i < len(docs)-1 {
			sections = append(sections, renderSidebarSeparator(contentWidth))
		}
	}
	hint := renderSidebarHint(contentWidth, m.sidebarFocused)
	content := lipgloss.JoinVertical(lipgloss.Left, sections...)
	lines := strings.Split(content, "\n")
	hintLines := strings.Split(hint, "\n")
	for len(lines)+len(hintLines) < height {
		lines = append(lines, "")
	}
	lines = append(lines, hintLines...)
	if len(lines) > height {
		lines = lines[:height]
	}
	for i := range lines {
		lines[i] = sidebarDividerStyle.Render("│") + "  " + fitLine(lines[i], contentWidth)
	}
	return strings.Join(lines, "\n")
}

func sidebarContentWidth(width int) int {
	return max(width-3, 10)
}

func renderSidebarSection(doc sidebarDocument, width int, height int, offset int, tag string, warn bool, focused bool) string {
	titleStyle := sidebarTitleStyle
	if focused {
		titleStyle = sidebarFocusedTitleStyle
	}
	tagStyle := statusDimStyle
	if warn || focused {
		tagStyle = planModeStyle
	}
	title := strings.ToUpper(doc.Title)
	pad := width - lipgloss.Width(title) - lipgloss.Width(tag)
	if pad < 1 {
		pad = 1
	}
	head := titleStyle.Render(title) + strings.Repeat(" ", pad) + tagStyle.Render(tag)

	text := doc.Content
	if doc.Error != "" {
		text = doc.Content + "\n" + doc.Error
	}
	rendered := xansi.Wrap(renderMarkdown(text, max(width, 20)), width, " ")
	lines := strings.Split(rendered, "\n")
	bodyLines := max(height-1, 1)
	offset = clampSidebarOffset(offset, len(lines), bodyLines)
	lines = sidebarVisibleLines(lines, offset, bodyLines)
	for len(lines) < bodyLines {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = mutedStyle.Render(lines[i])
	}
	return head + "\n" + strings.Join(lines, "\n")
}

func sidebarBoxesHeight(height int) int {
	return max(height-sidebarHintHeight, 1)
}

func sidebarDocumentAreaHeight(height int, count int) int {
	return max(sidebarBoxesHeight(height)-sidebarSeparatorsHeight(count), 1)
}

func sidebarSeparatorsHeight(count int) int {
	if count <= 1 {
		return 0
	}
	return (count - 1) * sidebarSeparatorHeight
}

func renderSidebarSeparator(width int) string {
	return sidebarDividerStyle.Render(strings.Repeat("┄", max(width, 1)))
}

func renderSidebarHint(width int, focused bool) string {
	text := "/sidebar off to hide"
	if focused {
		text = "Tab switch | Up/Down scroll | Esc"
		return hintStyle.Width(width).Render(fitLine(text, width))
	}
	return hintStyle.
		Width(width).
		Align(lipgloss.Right).
		Render(fitLine(text, width))
}

func sidebarBoxHeights(height int, count int) []int {
	if count <= 0 {
		return nil
	}
	boxHeight := max(height/count, 3)
	remainder := max(height-(boxHeight*count), 0)
	heights := make([]int, count)
	for i := range heights {
		heights[i] = boxHeight
		if i < remainder {
			heights[i]++
		}
	}
	return heights
}

func renderSidebarBox(doc sidebarDocument, width int, height int, offset int) string {
	return renderSidebarBoxWithFocus(doc, width, height, offset, false)
}

func renderSidebarBoxWithFocus(doc sidebarDocument, width int, height int, offset int, focused bool) string {
	contentWidth := max(width-sidebarBoxStyle.GetHorizontalFrameSize(), 10)
	contentHeight := max(height-sidebarBoxStyle.GetVerticalFrameSize(), 1)
	bodyWidth := contentWidth

	boxStyle := sidebarBoxStyle
	if focused {
		boxStyle = sidebarFocusedBoxStyle
	}
	title := strings.ToUpper(doc.Title)
	text := doc.Content
	if doc.Error != "" {
		text = doc.Content + "\n" + doc.Error
	}
	rendered := xansi.Wrap(renderMarkdown(text, max(bodyWidth, 20)), bodyWidth, " ")
	lines := strings.Split(rendered, "\n")
	availableBodyLines := max(contentHeight-2, 0)
	offset = clampSidebarOffset(offset, len(lines), availableBodyLines)
	if len(lines) > availableBodyLines {
		lines = sidebarVisibleLines(lines, offset, availableBodyLines)
	} else if availableBodyLines < len(lines) {
		lines = lines[:availableBodyLines]
	}
	for len(lines) < availableBodyLines {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = fitLine(lines[i], bodyWidth)
	}

	header := renderSidebarTitle(title, bodyWidth, focused)
	contentLines := []string{header, ""}
	contentLines = append(contentLines, lines...)
	for len(contentLines) < contentHeight {
		contentLines = append(contentLines, "")
	}
	content := strings.Join(contentLines, "\n")
	return boxStyle.
		Width(contentWidth).
		Height(contentHeight).
		Render(content)
}

func renderSidebarTitle(title string, width int, focused bool) string {
	titleStyle := sidebarTitleStyle
	if focused {
		titleStyle = sidebarFocusedTitleStyle
	}
	label := " " + strings.ToUpper(strings.TrimSpace(title)) + " "
	line := "─" + label
	fill := width - lipgloss.Width(line)
	if fill > 0 {
		line += strings.Repeat("─", fill)
	}
	return titleStyle.Render(fitLine(line, width))
}

func maxSidebarScrollOffset(doc sidebarDocument, width int, height int) int {
	contentWidth := max(width-sidebarBoxStyle.GetHorizontalFrameSize(), 10)
	contentHeight := max(height-sidebarBoxStyle.GetVerticalFrameSize(), 1)
	bodyWidth := contentWidth

	text := doc.Content
	if doc.Error != "" {
		text = doc.Content + "\n" + doc.Error
	}
	lines := strings.Split(xansi.Wrap(renderMarkdown(text, max(bodyWidth, 20)), bodyWidth, " "), "\n")
	availableBodyLines := max(contentHeight-2, 0)
	return clampSidebarOffset(len(lines), len(lines), availableBodyLines)
}

func clampSidebarOffset(offset int, lineCount int, availableBodyLines int) int {
	if offset < 0 || availableBodyLines <= 0 || lineCount <= availableBodyLines {
		return 0
	}
	maxOffset := lineCount - availableBodyLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		return maxOffset
	}
	return offset
}

func sidebarVisibleLines(lines []string, offset int, availableBodyLines int) []string {
	if availableBodyLines <= 0 {
		return nil
	}
	if len(lines) <= availableBodyLines {
		return lines
	}
	visibleContentLines := availableBodyLines
	offset = clampSidebarOffset(offset, len(lines), availableBodyLines)
	end := min(offset+visibleContentLines, len(lines))
	return append([]string(nil), lines[offset:end]...)
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
	if e.role == "system" {
		body = mutedStyle.Render(body)
	}
	return fitLine(label+" "+meta, width) + "\n" + body
}

func renderUserEntry(e entry, width int) string {
	bodyWidth := max(width-1, 20)
	body := wrapText(e.body, max(bodyWidth-2, 20))
	if body == "" {
		body = " "
	}
	return userBubbleStyle.Width(bodyWidth).Render(body)
}

func renderAssistantEntry(e entry, width int) string {
	return assistantLabelStyle.Width(width).Render(renderMarkdown(e.body, max(width, 20)))
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
	label := strings.TrimSpace(e.body)
	if label == "" {
		label = strings.TrimPrefix(strings.TrimSpace(e.title), "call ")
	}
	line := toolLabelStyle.Render("◆ " + label)
	return fitLine(line, width)
}

func renderToolResult(e entry, width int) string {
	output := strings.TrimSpace(e.body)
	if output == "" {
		output = "(no output)"
	}
	prefix := "└─ "
	bodyWidth := max(width-lipgloss.Width(prefix), 20)
	wrapped := wrapText(output, bodyWidth)
	lines := strings.Split(wrapped, "\n")
	for i := range lines {
		if i == 0 {
			lines[i] = prefix + lines[i]
			continue
		}
		lines[i] = strings.Repeat(" ", lipgloss.Width(prefix)) + lines[i]
	}
	style := mutedStyle
	if e.err {
		style = errorLabelStyle
	}
	return style.Width(width).Render(strings.Join(lines, "\n"))
}

func isToolResultPair(prev entry, current entry) bool {
	return prev.role == "tool" &&
		current.role == "tool" &&
		strings.HasPrefix(prev.title, "call ") &&
		strings.HasPrefix(current.title, "result ")
}

func (m Model) renderInput(width int) string {
	prompt := lipgloss.NewStyle().Foreground(cAccentHi).Render("> ")
	value := string(m.input)
	if value == "" {
		placeholder := "ask anything, or /help for commands"
		if m.running {
			placeholder = "message will be queued, or /cancel"
		}
		value = m.renderCursor() + " " + placeholderStyle.Render(placeholder)
	} else {
		value = strings.ReplaceAll(value, "\n", "\n  ")
		value = lipgloss.NewStyle().Foreground(cTextPri).Render(value) + m.renderCursor()
	}
	return inputStyle.Width(width - inputStyle.GetHorizontalFrameSize()).Render(prompt + value)
}

func (m Model) renderRunningNotice(width int) string {
	if !m.running {
		return ""
	}
	tag := planModeStyle.Bold(true).Render("[ WORKING ]")
	elapsed := mutedStyle.Render(fmt.Sprintf("elapsed %02ds", int(m.runningElapsed().Seconds())))
	hint := mutedStyle.Render("esc to interrupt")
	prefix := lipgloss.JoinHorizontal(lipgloss.Top, tag, " ", elapsed, " ")
	barWidth := width - lipgloss.Width(prefix) - lipgloss.Width(hint) - 1
	if barWidth < 1 {
		barWidth = 1
	}
	bar := renderWorkingBar(m.spinnerFrame, barWidth)
	left := prefix + bar
	pad := width - lipgloss.Width(left) - lipgloss.Width(hint)
	if pad < 1 {
		pad = 1
	}
	lines := []string{left + strings.Repeat(" ", pad) + hint}
	lines = append(lines, queuedPromptNoticeLines(m.queuedPrompts, width)...)
	return runningNoticeStyle.Width(width - runningNoticeStyle.GetHorizontalFrameSize()).Render(strings.Join(lines, "\n"))
}

func renderWorkingBar(frame int, width int) string {
	if width <= 0 {
		return ""
	}
	pos := frame % width
	activeWidth := min(14, width)
	var b strings.Builder
	for i := 0; i < width; i++ {
		if i >= pos && i < pos+activeWidth {
			b.WriteString(lipgloss.NewStyle().Foreground(cWarn).Render("▰"))
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(cProgressEmpty).Render("▱"))
		}
	}
	return b.String()
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
	return cursorStyle.Render("▌")
}

func (m Model) renderCursor() string {
	if m.spinnerFrame%2 == 1 {
		return " "
	}
	return renderCursor()
}

func (m Model) renderFooter(width int) string {
	help := "Enter send | Up/Down history | Tab complete | Shift+Tab plan | PgUp/PgDown/wheel scroll | Ctrl+F sidebar | Ctrl+C twice quit"
	if m.sidebarFocused {
		help = "Tab switch box | Up/Down/PgUp/PgDown scroll | 1/2/3 select | Esc close | Ctrl+C twice quit"
	} else if m.hasSlashMenu() {
		help = "Up/Down select | Tab complete | Enter run | Esc close | Ctrl+C twice quit"
	} else if m.hasFileMentionMenu() {
		help = "Up/Down select | Tab complete file | Enter send | Esc close | Ctrl+C twice quit"
	} else if m.running {
		help = "Enter queue | Shift+Tab toggles next run | Esc cancel current run | Ctrl+F sidebar | Ctrl+C twice quit"
	}
	line := fmt.Sprintf("%s  %s  %s", statusModelStyle.Render("fox"), statusDimStyle.Render("│"), m.status+"  "+help)
	return footerStyle.Width(width).Render(fitLine(line, width))
}

func (m Model) renderStatusBar(width int) string {
	parts := []string{
		statusModelStyle.Bold(true).Render("FOXHARNESS"),
		statusProjectStyle.Render(m.modelName),
		statusProjectStyle.Render(m.project),
		mutedStyle.Render("git ") + statusProjectStyle.Render(m.gitBranch),
		mutedStyle.Render("Context ") + statusModelStyle.Render(normalizeContextUsage(m.contextUsage)),
		mutedStyle.Render("sid ") + statusFaintStyle.Render(m.sessionID),
	}
	separator := " " + statusFaintStyle.Render("│") + " "
	return footerStyle.Width(width).Render(fitLine(strings.Join(parts, separator), width))
}

func (m Model) renderKeybinds(width int) string {
	plan := mutedStyle.Render("[ plan mode off ]")
	if m.planMode {
		plan = planModeStyle.Render("[ plan mode on ]")
	}
	hint := statusFaintStyle.Render("shift + tab to cycle")
	pad := width - lipgloss.Width(plan) - lipgloss.Width(hint)
	if pad < 1 {
		pad = 1
	}
	return footerStyle.Width(width).Render(fitLine(plan+strings.Repeat(" ", pad)+hint, width))
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
