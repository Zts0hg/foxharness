package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const (
	maxQueuedNoticeItems        = 3
	minTranscriptHeight         = 6
	maxCollapsedToolOutputLines = 3
	toolCallGlyph               = "⬢"
	workingNoticeText           = "working..."

	viewPaddingTop      = 1
	viewPaddingRight    = 2
	viewPaddingBottom   = 0
	viewPaddingLeft     = 2
	sidebarDividerWidth = 3

	amberBgHex            = "#0a0703"
	amberPanelHex         = "#171006"
	amberHex              = "#ffc56b"
	amberHiHex            = "#ffe3a8"
	amberWarnHex          = "#ff8855"
	amberMutedHex         = "#D69D60"
	amberDimHex           = "#B07E48"
	amberDividerHex       = "#6b4520"
	amberProgressEmptyHex = "#5a3020"
	claudeWorkingHex      = "#D77757"
	claudeShimmerHex      = "#F59575"
	selectionBgHex        = "#FFC56B"
	selectionFgHex        = "#1a0e03"
)

var (
	cBg             = lipgloss.Color(amberBgHex)
	cAccent         = lipgloss.Color(amberHex)
	cAccentHi       = lipgloss.Color(amberHiHex)
	cWarn           = lipgloss.Color(amberWarnHex)
	cTextPri        = lipgloss.Color(amberHiHex)
	cTextSec        = lipgloss.Color(amberHex)
	cTextMuted      = lipgloss.Color(amberMutedHex)
	cTextDim        = lipgloss.Color(amberDimHex)
	cTextVeryDim    = lipgloss.Color(amberDividerHex)
	cMsgBg          = lipgloss.Color(amberPanelHex)
	cProgressEmpty  = lipgloss.Color(amberProgressEmptyHex)
	cWorkingText    = lipgloss.Color(claudeWorkingHex)
	cWorkingShimmer = lipgloss.Color(claudeShimmerHex)
	cSelectionBg    = lipgloss.Color(selectionBgHex)
	cSelectionFg    = lipgloss.Color(selectionFgHex)

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
				Foreground(cWorkingText)
	workingGlyphStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(cWorkingText)
	workingTextStyle = lipgloss.NewStyle().
				Foreground(cWorkingText)
	workingShimmerStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(cWorkingShimmer)

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

	selectionStyle = lipgloss.NewStyle().
			Foreground(cSelectionFg).
			Background(cSelectionBg)

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
	if m.planForm != nil {
		formHeight := max(min(m.height/2, 18), 10)
		card := inputStyle.Width(width).Render(m.planForm.view(width, formHeight))
		chrome := outerStyle.GetVerticalFrameSize() +
			lipgloss.Height(card) + 1 +
			1 +
			lipgloss.Height(m.renderStatusBar(width)) +
			lipgloss.Height(m.renderKeybinds(width))
		bodyHeight := max(m.height-chrome, 1)
		parts := []string{
			m.renderMainArea(bodyHeight),
			"",
			card,
			"",
			m.renderStatusBar(width),
			m.renderKeybinds(width),
		}
		return outerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	if m.approvalForm != nil {
		card := inputStyle.Width(width).Render(m.approvalForm.view(width))
		chrome := outerStyle.GetVerticalFrameSize() +
			lipgloss.Height(card) + 1 +
			1 +
			lipgloss.Height(m.renderStatusBar(width)) +
			lipgloss.Height(m.renderKeybinds(width))
		bodyHeight := max(m.height-chrome, m.minTranscriptHeightForWindow())
		parts := []string{
			m.renderMainArea(bodyHeight),
			"",
			card,
			"",
			m.renderStatusBar(width),
			m.renderKeybinds(width),
		}
		return outerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	if m.permissionForm != nil {
		card := inputStyle.Width(width).Render(m.permissionForm.view(width))
		chrome := outerStyle.GetVerticalFrameSize() +
			lipgloss.Height(card) + 1 +
			1 +
			lipgloss.Height(m.renderStatusBar(width)) +
			lipgloss.Height(m.renderKeybinds(width))
		bodyHeight := max(m.height-chrome, m.minTranscriptHeightForWindow())
		parts := []string{
			m.renderMainArea(bodyHeight),
			"",
			card,
			"",
			m.renderStatusBar(width),
			m.renderKeybinds(width),
		}
		return outerStyle.Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	}

	if m.askForm != nil {
		// Render the question inline at the bottom (replacing the input band),
		// keeping the conversation transcript visible above — rather than a
		// full-screen takeover like the rewind selector.
		card := inputStyle.Width(width).Render(m.askForm.view(width))
		chrome := outerStyle.GetVerticalFrameSize() +
			lipgloss.Height(card) + 1 /* gap above card */ +
			1 /* footer gap */ +
			lipgloss.Height(m.renderStatusBar(width)) +
			lipgloss.Height(m.renderKeybinds(width))
		bodyHeight := max(m.height-chrome, m.minTranscriptHeightForWindow())
		parts := []string{
			m.renderMainArea(bodyHeight),
			"",
			card,
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
		parts = append(parts, "")
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
	height := max(m.height, 1)
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
		chrome += lipgloss.Height(notice) + 1
	}
	if suggestions := m.renderSuggestions(innerWidth); suggestions != "" {
		chrome += lipgloss.Height(suggestions)
	}
	bodyHeight := height - chrome
	minBodyHeight := m.minTranscriptHeightForWindow()
	if bodyHeight < minBodyHeight {
		bodyHeight = minBodyHeight
	}
	return contentWidth, bodyHeight
}

func (m Model) innerWidth() int {
	width := max(m.width, minWidth)
	inner := width - outerStyle.GetHorizontalFrameSize() - 1
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
	var layout transcriptLayout
	if m.cachedLayout != nil {
		layout = *m.cachedLayout
		visible := max(height-bodyStyle.GetVerticalFrameSize(), 1)
		start := len(layout.styledLines) - visible - m.scrollOffset
		if start < 0 {
			start = 0
		}
		layout.visibleStart = start
		layout.visibleEnd = min(start+visible, len(layout.styledLines))
	} else {
		layout = m.transcriptLayout(width, height)
	}
	lines := append([]string(nil), layout.styledLines[layout.visibleStart:layout.visibleEnd]...)
	m.applySelectionHighlight(lines, layout.visibleStart, selectionAreaTranscript)
	view := strings.Join(lines, "\n")
	return bodyStyle.Width(width).Height(height).Render(view)
}

type transcriptLayout struct {
	styledLines  []string
	plainLines   []string
	visibleStart int
	visibleEnd   int
}

func (m Model) transcriptLayout(width int, height int) transcriptLayout {
	contentWidth := max(width, 20)
	content := m.renderEntries(contentWidth)
	lines := strings.Split(content, "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = []string{placeholderStyle.Render("Start typing below. Use /help for commands.")}
	}
	plainLines := make([]string, len(lines))
	for i, line := range lines {
		plainLines[i] = xansi.Strip(line)
	}

	visible := max(height-bodyStyle.GetVerticalFrameSize(), 1)
	start := len(lines) - visible - m.scrollOffset
	if start < 0 {
		start = 0
	}
	end := min(start+visible, len(lines))
	return transcriptLayout{
		styledLines:  lines,
		plainLines:   plainLines,
		visibleStart: start,
		visibleEnd:   end,
	}
}

func (m Model) applySelectionHighlight(lines []string, absoluteStart int, area selectionArea) {
	if !m.selection.active || m.selectionArea() != area {
		return
	}
	start, end := normalizedSelection(m.selection)
	for i := range lines {
		lineNo := absoluteStart + i
		if lineNo < start.line || lineNo > end.line {
			continue
		}
		lineWidth := xansi.StringWidth(lines[i])
		left, right := 0, lineWidth
		if lineNo == start.line {
			left = min(max(start.col, 0), lineWidth)
		}
		if lineNo == end.line {
			right = min(max(end.col, 0), lineWidth)
		}
		if right <= left {
			continue
		}
		before := xansi.Cut(lines[i], 0, left)
		selected := xansi.Cut(xansi.Strip(lines[i]), left, right)
		after := xansi.Cut(lines[i], right, lineWidth)
		lines[i] = before + selectionStyle.Render(selected) + after
	}
}

func (m Model) selectionArea() selectionArea {
	if m.selection.area == selectionAreaSidebar {
		return selectionAreaSidebar
	}
	if m.selection.area == selectionAreaInput {
		return selectionAreaInput
	}
	return selectionAreaTranscript
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
	layout := m.sidebarLayout(width, height)
	lines := append([]string(nil), layout.styledLines...)
	m.applySelectionHighlight(lines, 0, selectionAreaSidebar)
	for i := range lines {
		lines[i] = sidebarDividerStyle.Render("│") + "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

type sidebarLayout struct {
	styledLines []string
	plainLines  []string
	width       int
}

func (m Model) sidebarLayout(width int, height int) sidebarLayout {
	docs := m.sidebarDocuments
	if len(docs) == 0 {
		docs = loadSidebarDocuments(m.runner.WorkDir(), m.runner.SessionDir(), m.runner.AutoMemoryIndex())
	}
	if len(docs) == 0 {
		return sidebarLayout{}
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
	plainLines := make([]string, len(lines))
	for i := range lines {
		lines[i] = fitLine(lines[i], contentWidth)
		plainLines[i] = xansi.Strip(lines[i])
	}
	return sidebarLayout{
		styledLines: lines,
		plainLines:  plainLines,
		width:       contentWidth,
	}
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
	lines := sidebarDocumentLines(text, width)
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
	lines := sidebarDocumentLines(sidebarDisplayContent(doc), bodyWidth)
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

	lines := sidebarDocumentLines(sidebarDisplayContent(doc), bodyWidth)
	availableBodyLines := max(contentHeight-2, 0)
	return clampSidebarOffset(len(lines), len(lines), availableBodyLines)
}

func sidebarDisplayContent(doc sidebarDocument) string {
	content := trimSidebarRedundantHeading(doc.Title, doc.Content)
	if doc.Error != "" {
		return content + "\n" + doc.Error
	}
	return content
}

func trimSidebarRedundantHeading(title string, content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || !isSidebarRedundantHeading(title, lines[0]) {
		return content
	}

	start := 1
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	return strings.Join(lines[start:], "\n")
}

func isSidebarRedundantHeading(title string, line string) bool {
	heading := strings.TrimSpace(line)
	return heading == "# "+strings.TrimSpace(title) || heading == "# "+strings.ToUpper(strings.TrimSpace(title))
}

func sidebarDocumentLines(text string, width int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return []string{""}
	}
	width = max(width, 10)

	var lines []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimRight(raw, " \t")
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		if prefix, body, ok := sidebarListPrefix(line); ok {
			lines = append(lines, wrapSidebarPrefixedLine(prefix, body, width)...)
			continue
		}
		lines = append(lines, wrapPlainLine(strings.TrimSpace(line), width)...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func sidebarListPrefix(line string) (string, string, bool) {
	indentLen := len(line) - len(strings.TrimLeft(line, " \t"))
	indent := strings.Repeat(" ", min(indentLen, 8))
	trimmed := strings.TrimLeft(line, " \t")

	for _, marker := range []string{"- [ ] ", "* [ ] "} {
		if strings.HasPrefix(trimmed, marker) {
			return indent + "[ ] ", strings.TrimSpace(strings.TrimPrefix(trimmed, marker)), true
		}
	}
	for _, marker := range []string{"- [x] ", "- [X] ", "* [x] ", "* [X] "} {
		if strings.HasPrefix(trimmed, marker) {
			return indent + "[✓] ", strings.TrimSpace(trimmed[len(marker):]), true
		}
	}
	for _, marker := range []string{"- ", "* "} {
		if strings.HasPrefix(trimmed, marker) {
			return indent + "• ", strings.TrimSpace(strings.TrimPrefix(trimmed, marker)), true
		}
	}

	dot := strings.IndexByte(trimmed, '.')
	if dot <= 0 || dot+1 >= len(trimmed) || trimmed[dot+1] != ' ' {
		return "", "", false
	}
	for _, r := range trimmed[:dot] {
		if r < '0' || r > '9' {
			return "", "", false
		}
	}
	return indent + trimmed[:dot+2], strings.TrimSpace(trimmed[dot+2:]), true
}

func wrapSidebarPrefixedLine(prefix string, body string, width int) []string {
	prefix = fitLine(prefix, max(width-1, 1))
	prefixWidth := lipgloss.Width(prefix)
	body = strings.TrimSpace(body)
	if body == "" {
		return []string{fitLine(prefix, width)}
	}

	continuation := strings.Repeat(" ", min(prefixWidth, max(width-1, 1)))
	var lines []string
	currentPrefix := prefix
	remaining := body
	for {
		available := max(width-lipgloss.Width(currentPrefix), 1)
		head, tail := splitLineAtWidth(remaining, available)
		if strings.TrimSpace(head) == "" {
			head, tail = splitLineAtWidth(remaining, max(available, 2))
		}
		lines = append(lines, fitLine(currentPrefix+head, width))
		if strings.TrimSpace(tail) == "" {
			break
		}
		currentPrefix = continuation
		remaining = strings.TrimSpace(tail)
	}
	return lines
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
		out.WriteString(renderEntryWithOptions(e, width, m.toolOutputExpanded))
	}
	return out.String()
}

func renderEntry(e entry, width int) string {
	return renderEntryWithOptions(e, width, true)
}

func renderEntryWithOptions(e entry, width int, toolOutputExpanded bool) string {
	switch {
	case e.role == "user":
		return renderUserEntry(e, width)
	case e.role == "tool" && strings.HasPrefix(e.title, "call "):
		return renderToolCall(e, width)
	case e.role == "tool" && strings.HasPrefix(e.title, "result "):
		return renderToolResult(e, width, toolOutputExpanded)
	case e.role == "assistant":
		return renderAssistantEntry(e, width)
	case e.role == "command":
		return renderCommandEntry(e, width, toolOutputExpanded)
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
	body := renderPlainBlock(e.body, max(bodyWidth-2, 20))
	if body == "" {
		body = " "
	}
	return userBubbleStyle.Width(bodyWidth).Render(body)
}

func renderAssistantEntry(e entry, width int) string {
	return assistantLabelStyle.Width(width).Render(renderMarkdown(e.body, max(width, 20)))
}

func renderCommandEntry(e entry, width int, expanded bool) string {
	title := strings.TrimSpace(e.title)
	if title == "" {
		title = "Result"
	}
	headerStyle := commandLabelStyle
	if e.err {
		headerStyle = errorLabelStyle
		title = "FAILED " + title
	}
	header := fitLine(headerStyle.Render(title), width)
	bodyWidth := max(width-2, 20)
	var body string
	if isShellCommandEntry(e) {
		body = renderPlainBlockPreserveWhitespace(e.body, bodyWidth)
	} else {
		body = renderPlainBlock(e.body, bodyWidth)
	}
	if body == "" {
		return header
	}
	if isShellCommandEntry(e) {
		body = collapseCommandOutput(body, expanded)
	}
	return header + "\n" + indentLines(body, "  ")
}

func isShellCommandEntry(e entry) bool {
	return e.role == "command" && strings.HasPrefix(strings.TrimSpace(e.title), "Shell: !")
}

func collapseCommandOutput(body string, expanded bool) string {
	if expanded {
		return body
	}
	lines := strings.Split(body, "\n")
	if len(lines) <= maxCollapsedToolOutputLines {
		return body
	}
	hidden := len(lines) - maxCollapsedToolOutputLines
	lines = append(lines[:maxCollapsedToolOutputLines], fmt.Sprintf("+%d lines (ctrl+o to expand)", hidden))
	return strings.Join(lines, "\n")
}

func renderToolCall(e entry, width int) string {
	label := strings.TrimSpace(e.body)
	if label == "" {
		label = strings.TrimPrefix(strings.TrimSpace(e.title), "call ")
	}
	line := toolLabelStyle.Render(toolCallGlyph + " " + label)
	return fitLine(line, width)
}

func renderToolResult(e entry, width int, expanded bool) string {
	output := strings.TrimSpace(e.body)
	if output == "" {
		output = "(no output)"
	}
	prefix := "└─ "
	bodyWidth := max(width-lipgloss.Width(prefix), 20)
	wrapped := wrapText(output, bodyWidth)
	lines := strings.Split(wrapped, "\n")
	if !expanded && len(lines) > maxCollapsedToolOutputLines {
		hidden := len(lines) - maxCollapsedToolOutputLines
		lines = append(lines[:maxCollapsedToolOutputLines], fmt.Sprintf("+%d lines (ctrl+o to expand)", hidden))
	}
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
	bang := m.hasBangPrefix() && m.inputPastePreviewLabel() == ""
	promptText := inputPromptText(m)
	prompt := lipgloss.NewStyle().Foreground(cAccentHi).Render(promptText)
	if bang {
		prompt = lipgloss.NewStyle().Bold(true).Foreground(cWarn).Render(promptText)
	}

	// In bang mode the leading '!' is absorbed into the prompt, so render
	// against a display copy with that rune removed; the cursor index shifts by
	// one to stay aligned with the command text. The editing state is untouched.
	view := m
	if bang {
		view.input = m.input[1:]
		if view.inputCursor > 0 {
			view.inputCursor--
		}
	}

	value := string(view.input)
	cursor := ""
	if view.inputCanAcceptTyping() {
		cursor = view.renderCursor()
	}
	if value == "" {
		placeholder := "ask anything, or /help for commands"
		if bang {
			placeholder = "run a local shell command, e.g. ls"
		}
		if m.running {
			placeholder = "message will be queued, or /cancel"
		}
		if cursor != "" {
			value = cursor + " " + placeholderStyle.Render(placeholder)
		} else {
			value = placeholderStyle.Render(placeholder)
		}
	} else {
		if label := m.inputPastePreviewLabel(); label != "" && len(m.input) > 0 {
			return inputStyle.Width(width - inputStyle.GetHorizontalFrameSize()).Render(prompt + lipgloss.NewStyle().Foreground(cTextPri).Render(label) + cursor)
		}
		view.clampInputCursor()
		return inputStyle.Width(width - inputStyle.GetHorizontalFrameSize()).Render(view.renderInputRows(prompt, cursor))
	}
	return inputStyle.Width(width - inputStyle.GetHorizontalFrameSize()).Render(prompt + value)
}

func (m Model) renderInputRows(prompt string, cursor string) string {
	rows := m.inputRenderRows()
	displayRows := m.inputDisplayRows(rows)
	lines := make([]string, 0, len(displayRows))
	textStyle := lipgloss.NewStyle().Foreground(cTextPri)
	argHint := m.activeSlashArgumentHint()
	for i, displayRow := range displayRows {
		prefix := "  "
		if i == 0 {
			prefix = prompt
		}
		if displayRow.marker != "" {
			lines = append(lines, prefix+mutedStyle.Render(displayRow.marker))
			continue
		}
		row := displayRow.row
		if m.selection.active && m.selection.area == selectionAreaInput {
			lines = append(lines, prefix+m.renderInputRowText(row, displayRow.index, textStyle))
			continue
		}
		hasCursor := cursor != "" && cursor != " " && m.inputCursor >= row.start && m.inputCursor <= row.end
		if hasCursor && displayRow.index+1 < len(rows) && m.inputCursor == row.end && rows[displayRow.index+1].start == m.inputCursor {
			hasCursor = false
		}
		if hasCursor {
			before := string(m.input[row.start:m.inputCursor])
			cursorCell := cursor
			afterStart := m.inputCursor
			if cursor != " " && afterStart < row.end {
				cursorCell = renderCursorOverRune(m.input[afterStart])
				afterStart++
			}
			after := string(m.input[afterStart:row.end])
			hint := ""
			if argHint != "" && m.inputCursor == len(m.input) && after == "" {
				remaining := m.inputTextWidth() - xansi.StringWidth(before) - xansi.StringWidth(xansi.Strip(cursorCell))
				if remaining > 0 {
					hint = hintStyle.Render(xansi.Truncate(argHint, remaining, ""))
				}
			}
			lines = append(lines, prefix+textStyle.Render(before)+cursorCell+hint+textStyle.Render(after))
		} else {
			lines = append(lines, prefix+textStyle.Render(string(m.input[row.start:row.end])))
		}
	}
	return strings.Join(lines, "\n")
}

func inputPromptText(m Model) string {
	if m.hasBangPrefix() && m.inputPastePreviewLabel() == "" {
		return "! "
	}
	return "> "
}

func (m Model) renderInputRowText(row inputRenderRow, rowIndex int, textStyle lipgloss.Style) string {
	text := string(m.input[row.start:row.end])
	if !m.selection.active || m.selection.area != selectionAreaInput {
		return textStyle.Render(text)
	}
	start, end := normalizedSelection(m.selection)
	if rowIndex < start.line || rowIndex > end.line {
		return textStyle.Render(text)
	}
	width := xansi.StringWidth(text)
	left, right := 0, width
	if rowIndex == start.line {
		left = min(max(start.col, 0), width)
	}
	if rowIndex == end.line {
		right = min(max(end.col, 0), width)
	}
	if right <= left {
		return textStyle.Render(text)
	}
	before := xansi.Cut(text, 0, left)
	selected := xansi.Cut(text, left, right)
	after := xansi.Cut(text, right, width)
	return textStyle.Render(before) + selectionStyle.Render(selected) + textStyle.Render(after)
}

func (m Model) inputContentY() int {
	_, bodyHeight := m.contentDimensions()
	y := viewPaddingTop + bodyHeight
	if notice := m.renderRunningNotice(m.innerWidth()); notice != "" {
		y += 1 + lipgloss.Height(notice)
	}
	return y + inputStyle.GetVerticalFrameSize()/2
}

type inputDisplayRow struct {
	row    inputRenderRow
	index  int
	marker string
}

func (m Model) inputDisplayRows(rows []inputRenderRow) []inputDisplayRow {
	maxRows := m.maxVisibleInputRows()
	if len(rows) <= maxRows {
		out := make([]inputDisplayRow, 0, len(rows))
		for i, row := range rows {
			out = append(out, inputDisplayRow{row: row, index: i})
		}
		return out
	}
	if maxRows <= 1 {
		return []inputDisplayRow{{row: rows[len(rows)-1], index: len(rows) - 1}}
	}
	if maxRows == 2 {
		return []inputDisplayRow{
			{marker: inputHiddenRowsLabel(len(rows) - 1)},
			{row: rows[len(rows)-1], index: len(rows) - 1},
		}
	}

	headCount := max(1, (maxRows-1)/2)
	tailCount := maxRows - headCount - 1
	if tailCount < 1 {
		tailCount = 1
		headCount = maxRows - tailCount - 1
	}
	tailStart := len(rows) - tailCount
	if tailStart <= headCount {
		out := make([]inputDisplayRow, 0, maxRows)
		for i, row := range rows[:maxRows] {
			out = append(out, inputDisplayRow{row: row, index: i})
		}
		return out
	}

	out := make([]inputDisplayRow, 0, maxRows)
	for i, row := range rows[:headCount] {
		out = append(out, inputDisplayRow{row: row, index: i})
	}
	out = append(out, inputDisplayRow{marker: inputHiddenRowsLabel(tailStart - headCount)})
	for i, row := range rows[tailStart:] {
		out = append(out, inputDisplayRow{row: row, index: tailStart + i})
	}
	return out
}

func inputHiddenRowsLabel(count int) string {
	if count == 1 {
		return "[... 1 line hidden ...]"
	}
	return fmt.Sprintf("[... %d lines hidden ...]", count)
}

func (m Model) maxVisibleInputRows() int {
	height := max(m.height, 1)
	innerWidth := m.innerWidth()
	chrome := outerStyle.GetVerticalFrameSize() +
		inputStyle.GetVerticalFrameSize() +
		1 /* footer gap */ +
		lipgloss.Height(m.renderStatusBar(innerWidth)) +
		lipgloss.Height(m.renderKeybinds(innerWidth)) +
		m.minTranscriptHeightForWindow()
	if notice := m.renderRunningNotice(innerWidth); notice != "" {
		chrome += lipgloss.Height(notice) + 1
	}
	if suggestions := m.renderSuggestions(innerWidth); suggestions != "" {
		chrome += lipgloss.Height(suggestions)
	}
	available := height - chrome
	if available < 1 {
		return 1
	}
	return available
}

func (m Model) minTranscriptHeightForWindow() int {
	height := max(m.height, 1)
	innerWidth := m.innerWidth()
	chromeWithOneInputRow := outerStyle.GetVerticalFrameSize() +
		inputStyle.GetVerticalFrameSize() +
		1 /* visible input row */ +
		1 /* footer gap */ +
		lipgloss.Height(m.renderStatusBar(innerWidth)) +
		lipgloss.Height(m.renderKeybinds(innerWidth))
	if notice := m.renderRunningNotice(innerWidth); notice != "" {
		chromeWithOneInputRow += lipgloss.Height(notice) + 1
	}
	if suggestions := m.renderSuggestions(innerWidth); suggestions != "" {
		chromeWithOneInputRow += lipgloss.Height(suggestions)
	}
	available := height - chromeWithOneInputRow
	if available < 1 {
		return 1
	}
	return min(minTranscriptHeight, available)
}

func (m Model) inputCanAcceptTyping() bool {
	return m.terminalFocused && !m.sidebarFocused
}

func (m Model) renderRunningNotice(width int) string {
	if !m.running {
		return ""
	}
	glyph := workingGlyphStyle.Render(m.workingFrame())
	text := renderWorkingText(m.spinnerFrame)
	hint := mutedStyle.Render(formatDuration(m.runningElapsed()) + " • esc to interrupt")
	lines := []string{lipgloss.JoinHorizontal(lipgloss.Top, glyph, " ", text, " ", hint)}
	lines = append(lines, queuedPromptNoticeLines(m.queuedPrompts, width)...)
	return runningNoticeStyle.Width(width - runningNoticeStyle.GetHorizontalFrameSize()).Render(strings.Join(lines, "\n"))
}

func renderWorkingText(frame int) string {
	before, shimmer, after := workingShimmerSegments(workingNoticeText, workingGlimmerIndex(frame, len([]rune(workingNoticeText))))
	return workingTextStyle.Render(before) +
		workingShimmerStyle.Render(shimmer) +
		workingTextStyle.Render(after)
}

func workingGlimmerIndex(frame int, textWidth int) int {
	if textWidth <= 0 {
		return -100
	}
	cycleLength := textWidth + 20
	position := frame % cycleLength
	if position < 0 {
		position += cycleLength
	}
	return position - 10
}

func workingShimmerSegments(text string, glimmerIndex int) (string, string, string) {
	runes := []rune(text)
	shimmerStart := glimmerIndex - 1
	shimmerEnd := glimmerIndex + 1
	if shimmerStart >= len(runes) || shimmerEnd < 0 {
		return text, "", ""
	}
	start := max(shimmerStart, 0)
	end := min(shimmerEnd+1, len(runes))
	return string(runes[:start]), string(runes[start:end]), string(runes[end:])
}

func queuedPromptNoticeLines(prompts []queuedPrompt, width int) []string {
	if len(prompts) == 0 {
		return nil
	}
	lineWidth := max(width-runningNoticeStyle.GetHorizontalFrameSize()-2, 20)
	count := min(len(prompts), maxQueuedNoticeItems)
	lines := make([]string, 0, count+1)
	for i := 0; i < count; i++ {
		prefix := fmt.Sprintf("  %d. ", i+1)
		messageWidth := max(lineWidth-lipgloss.Width(prefix), 1)
		message := strings.Join(strings.Fields(prompts[i].text), " ")
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

func renderCursorOverRune(r rune) string {
	return lipgloss.NewStyle().
		Foreground(cBg).
		Background(cAccentHi).
		Render(string(r))
}

func (m Model) renderCursor() string {
	return renderCursor()
}

func (m Model) renderFooter(width int) string {
	help := "Enter send | !cmd shell | Up/Down history | Tab complete | Shift+Tab plan | PgUp/PgDown/wheel scroll | drag select to copy | Ctrl+F sidebar | Ctrl+C twice quit"
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
	items := m.statuslineItems
	if len(items) == 0 {
		items = defaultStatuslineItems
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		part := m.renderStatuslineItem(item)
		if strings.TrimSpace(xansi.Strip(part)) == "" {
			continue
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		parts = append(parts, statusProjectStyle.Render(m.modelName))
	}
	separator := " " + statusFaintStyle.Render("│") + " "
	return footerStyle.Width(width).Render(fitLine(strings.Join(parts, separator), width))
}

func (m Model) renderKeybinds(width int) string {
	plan := mutedStyle.Render("[ plan mode off ]")
	if m.collaborationMode.PlanEnabled() {
		plan = planModeStyle.Render("[ plan mode on ]")
	}
	if m.permissionSnapshot.EffectiveMode == permission.ModeFullAccess {
		plan += " " + planModeStyle.Render("[ full access ]")
	}
	hint := statusFaintStyle.Render("shift + tab to cycle")
	pad := width - lipgloss.Width(plan) - lipgloss.Width(hint)
	if pad < 1 {
		pad = 1
	}
	return footerStyle.Width(width).Render(fitLine(plan+strings.Repeat(" ", pad)+hint, width))
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
	return xansi.Truncate(s, max(width, 0), "...")
}

func formatDuration(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	if total < 0 {
		total = 0
	}
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
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

func renderPlainBlockPreserveWhitespace(text string, width int) string {
	if text == "" {
		return ""
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		out = append(out, wrapPlainLinePreserveWhitespace(line, width)...)
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

func wrapPlainLinePreserveWhitespace(line string, width int) []string {
	if line == "" {
		return []string{""}
	}
	if lipgloss.Width(line) <= width {
		return []string{line}
	}
	var lines []string
	current := line
	for lipgloss.Width(current) > width {
		head, tail := splitLineAtExactWidth(current, width)
		if head == "" {
			break
		}
		lines = append(lines, head)
		current = tail
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

func splitLineAtExactWidth(line string, width int) (string, string) {
	runes := []rune(line)
	for i := range runes {
		candidate := string(runes[:i+1])
		if lipgloss.Width(candidate) > width {
			if i == 0 {
				return string(runes[:1]), string(runes[1:])
			}
			return string(runes[:i]), string(runes[i:])
		}
		if lipgloss.Width(candidate) == width {
			return candidate, string(runes[i+1:])
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
