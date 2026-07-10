package tui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.GFM))
var markdownStyleRenderer = newMarkdownStyleRenderer()

var localLinkLocationSuffixRE = regexp.MustCompile(`:\d+(?::\d+)?(?:[-–]\d+(?::\d+)?)?$`)
var markdownFenceOpenRE = regexp.MustCompile(`^\s*(` + "`{3,}|~{3,}" + `)\s*([A-Za-z0-9_-]+)?\s*$`)

func applyMarkdownTheme(theme tuiTheme) {
	_ = theme
}

func newMarkdownStyleRenderer() *lipgloss.Renderer {
	renderer := lipgloss.NewRenderer(io.Discard)
	renderer.SetColorProfile(termenv.TrueColor)
	return renderer
}

func renderMarkdown(markdown string, width int) string {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return ""
	}
	width = max(width, 20)
	normalized := unwrapMarkdownTableFences(markdown)
	renderer := codexMarkdownRenderer{
		source: []byte(normalized),
		width:  width,
		cwd:    currentMarkdownCWD(),
	}
	reader := text.NewReader(renderer.source)
	doc := markdownParser.Parser().Parse(reader)
	renderer.renderChildren(doc, markdownContext{})
	return strings.TrimRight(strings.Join(renderer.lines, "\n"), "\n")
}

type codexMarkdownRenderer struct {
	source []byte
	width  int
	cwd    string
	lines  []string
}

type markdownContext struct {
	quotePrefix string
	listDepth   int
	firstPrefix string
	restPrefix  string
}

func (r *codexMarkdownRenderer) renderChildren(parent gast.Node, ctx markdownContext) {
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		r.renderBlock(child, ctx)
	}
}

func (r *codexMarkdownRenderer) renderBlock(node gast.Node, ctx markdownContext) {
	switch n := node.(type) {
	case *gast.Document:
		r.renderChildren(n, ctx)
	case *gast.Paragraph:
		r.renderParagraph(n, ctx)
	case *gast.Heading:
		r.renderHeading(n, ctx)
	case *gast.Blockquote:
		r.renderBlockquote(n, ctx)
	case *gast.List:
		r.renderList(n, ctx)
	case *gast.ListItem:
		r.renderListItem(n, ctx, "- ", 0)
	case *gast.ThematicBreak:
		r.renderThematicBreak(ctx)
	case *gast.FencedCodeBlock:
		r.renderCodeBlock(n.Text(r.source), ctx, string(n.Language(r.source)))
	case *gast.CodeBlock:
		r.renderCodeBlock(n.Text(r.source), ctx, "")
	case *gast.TextBlock:
		r.renderParagraph(n, ctx)
	case *gast.HTMLBlock:
		r.renderPlainText(string(n.Text(r.source)), ctx)
	default:
		if node.Kind() == east.KindTable {
			r.renderTable(node, ctx)
			return
		}
		r.renderChildren(node, ctx)
	}
}

func (r *codexMarkdownRenderer) renderParagraph(node gast.Node, ctx markdownContext) {
	content := strings.TrimSpace(r.renderInlineChildren(node, inlineStyle{}))
	if content == "" {
		return
	}
	r.pushWrapped(content, ctx)
}

func (r *codexMarkdownRenderer) renderHeading(node *gast.Heading, ctx markdownContext) {
	marker := strings.Repeat("#", node.Level) + " "
	content := marker + strings.TrimSpace(r.renderInlineChildren(node, inlineStyle{}))
	style := markdownHeadingStyle(node.Level)
	r.pushLine(ctx.quotePrefix + style.Render(content))
	r.ensureBlankAfterBlock()
}

func (r *codexMarkdownRenderer) renderBlockquote(node gast.Node, ctx markdownContext) {
	next := ctx
	next.quotePrefix += "> "
	next.firstPrefix = ""
	next.restPrefix = ""
	r.renderChildren(node, next)
}

func (r *codexMarkdownRenderer) renderList(list *gast.List, ctx markdownContext) {
	index := list.Start
	if index == 0 {
		index = 1
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "- "
		if list.IsOrdered() {
			marker = strconv.Itoa(index) + ". "
			index++
		}
		r.renderListItem(item, ctx, marker, ctx.listDepth)
	}
}

func (r *codexMarkdownRenderer) renderListItem(item gast.Node, ctx markdownContext, marker string, depth int) {
	baseIndent := strings.Repeat(" ", depth*4)
	firstPrefix := ctx.quotePrefix + baseIndent + marker
	restPrefix := ctx.quotePrefix + baseIndent + strings.Repeat(" ", lipgloss.Width(marker))
	childCtx := ctx
	childCtx.listDepth = depth
	childCtx.firstPrefix = firstPrefix
	childCtx.restPrefix = restPrefix

	first := true
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch c := child.(type) {
		case *gast.List:
			nested := ctx
			nested.listDepth = depth + 1
			r.renderList(c, nested)
		default:
			if first {
				r.renderBlock(child, childCtx)
				first = false
				continue
			}
			nextCtx := childCtx
			nextCtx.firstPrefix = restPrefix
			r.renderBlock(child, nextCtx)
		}
	}
}

func (r *codexMarkdownRenderer) renderThematicBreak(ctx markdownContext) {
	available := max(r.width-lipgloss.Width(ctx.quotePrefix), 8)
	r.pushLine(ctx.quotePrefix + markdownMutedStyle().Render(strings.Repeat("-", min(available, 80))))
	r.ensureBlankAfterBlock()
}

func (r *codexMarkdownRenderer) renderCodeBlock(content []byte, ctx markdownContext, lang string) {
	_ = lang
	text := strings.TrimRight(string(content), "\n")
	if text == "" {
		return
	}
	prefix := ctx.firstPrefix
	if prefix == "" {
		prefix = ctx.quotePrefix
	}
	rest := ctx.restPrefix
	if rest == "" {
		rest = ctx.quotePrefix
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		linePrefix := rest
		if i == 0 {
			linePrefix = prefix
		}
		r.pushLine(linePrefix + markdownCodeBlockStyle().Render(line))
	}
	r.ensureBlankAfterBlock()
}

func (r *codexMarkdownRenderer) renderPlainText(content string, ctx markdownContext) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	for _, line := range strings.Split(content, "\n") {
		r.pushWrapped(line, ctx)
	}
}

func (r *codexMarkdownRenderer) renderInlineChildren(node gast.Node, style inlineStyle) string {
	var out strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		out.WriteString(r.renderInline(child, style))
	}
	return out.String()
}

func (r *codexMarkdownRenderer) renderInline(node gast.Node, style inlineStyle) string {
	switch n := node.(type) {
	case *gast.Text:
		text := string(n.Value(r.source))
		text = strings.ReplaceAll(text, "\n", " ")
		if n.SoftLineBreak() || n.HardLineBreak() {
			text += "\n"
		}
		return style.apply(text)
	case *gast.String:
		return style.apply(string(n.Value))
	case *gast.CodeSpan:
		content := strings.TrimSpace(r.renderInlineChildren(n, inlineStyle{}))
		return renderStyledText(markdownInlineCodeStyle(), content)
	case *gast.Emphasis:
		next := style
		if n.Level >= 2 {
			next.bold = true
		} else {
			next.italic = true
		}
		return r.renderInlineChildren(n, next)
	case *gast.Link:
		label := strings.TrimSpace(r.renderInlineChildren(n, style))
		dest := string(n.Destination)
		if display, ok := r.localLinkDisplay(dest); ok {
			return renderStyledText(markdownLinkStyle(), display)
		}
		if label == "" {
			label = dest
		}
		return label + " (" + renderStyledText(markdownLinkStyle(), dest) + ")"
	default:
		if task, ok := node.(*east.TaskCheckBox); ok {
			if task.IsChecked {
				return "[x] "
			}
			return "[ ] "
		}
		if node.Kind() == east.KindStrikethrough {
			next := style
			next.strike = true
			return r.renderInlineChildren(node, next)
		}
		return r.renderInlineChildren(node, style)
	}
}

type inlineStyle struct {
	bold   bool
	italic bool
	strike bool
}

func (s inlineStyle) apply(text string) string {
	if !s.bold && !s.italic && !s.strike {
		return text
	}
	style := markdownStyleRenderer.NewStyle()
	if s.bold || s.italic || s.strike {
		style = style.Foreground(cAccentHi)
	}
	if s.bold {
		style = style.Bold(true)
	}
	if s.italic {
		style = style.Italic(true)
	}
	if s.strike {
		style = style.Strikethrough(true)
	}
	return renderStyledText(style, text)
}

func renderStyledText(style lipgloss.Style, text string) string {
	var out strings.Builder
	var token strings.Builder
	flush := func() {
		if token.Len() == 0 {
			return
		}
		out.WriteString(style.Render(token.String()))
		token.Reset()
	}
	for _, r := range text {
		if unicode.IsSpace(r) {
			flush()
			out.WriteRune(r)
			continue
		}
		token.WriteRune(r)
	}
	flush()
	return out.String()
}

func (r *codexMarkdownRenderer) pushWrapped(content string, ctx markdownContext) {
	prefix := ctx.firstPrefix
	if prefix == "" {
		prefix = ctx.quotePrefix
	}
	rest := ctx.restPrefix
	if rest == "" {
		rest = ctx.quotePrefix
	}
	available := max(r.width-lipgloss.Width(prefix), 8)
	lines := wrapANSIText(content, available)
	if len(lines) == 0 {
		r.pushLine(prefix)
		return
	}
	for i, line := range lines {
		linePrefix := rest
		if i == 0 {
			linePrefix = prefix
		}
		r.pushLine(linePrefix + line)
	}
}

func (r *codexMarkdownRenderer) pushLine(line string) {
	r.lines = append(r.lines, line)
}

func (r *codexMarkdownRenderer) ensureBlankAfterBlock() {
	if len(r.lines) == 0 {
		return
	}
	if strings.TrimSpace(xansi.Strip(r.lines[len(r.lines)-1])) == "" {
		return
	}
	r.lines = append(r.lines, "")
}

func (r *codexMarkdownRenderer) renderTable(node gast.Node, ctx markdownContext) {
	header, rows := r.collectTable(node)
	if len(header) == 0 {
		return
	}
	if len(rows) == 0 || r.tableNeedsKeyValue(header, rows) {
		r.renderKeyValueTable(header, rows, ctx)
		return
	}
	r.renderGridTable(header, rows, ctx)
}

func (r *codexMarkdownRenderer) collectTable(table gast.Node) ([]string, [][]string) {
	var header []string
	var rows [][]string
	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case east.KindTableHeader:
			header = r.collectTableRow(child)
		case east.KindTableRow:
			rows = append(rows, r.collectTableRow(child))
		}
	}
	return header, rows
}

func (r *codexMarkdownRenderer) collectTableRow(row gast.Node) []string {
	var cells []string
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		if cell.Kind() != east.KindTableCell {
			continue
		}
		text := strings.TrimSpace(r.renderInlineChildren(cell, inlineStyle{}))
		text = strings.Join(strings.Fields(text), " ")
		cells = append(cells, text)
	}
	return cells
}

func (r *codexMarkdownRenderer) tableNeedsKeyValue(header []string, rows [][]string) bool {
	widths := tableColumnWidths(header, rows)
	total := 0
	for i, w := range widths {
		total += w
		if i > 0 {
			total += 2
		}
	}
	if total <= r.width {
		return false
	}
	if r.width < 64 {
		return true
	}
	return total > r.width*2
}

func (r *codexMarkdownRenderer) renderGridTable(header []string, rows [][]string, ctx markdownContext) {
	widths := tableColumnWidths(header, rows)
	r.pushLine(ctx.quotePrefix + renderTableRow(header, widths, markdownTableHeaderStyle()))
	r.pushLine(ctx.quotePrefix + renderTableSeparator(widths, "━"))
	for i, row := range rows {
		for _, line := range renderWrappedTableRow(row, widths) {
			r.pushLine(ctx.quotePrefix + line)
		}
		if i < len(rows)-1 {
			r.pushLine(ctx.quotePrefix + renderTableSeparator(widths, "─"))
		}
	}
	r.ensureBlankAfterBlock()
}

func (r *codexMarkdownRenderer) renderKeyValueTable(header []string, rows [][]string, ctx markdownContext) {
	valueWidth := max(r.width-lipgloss.Width(ctx.quotePrefix)-2, 10)
	for rowIndex, row := range rows {
		for i, label := range header {
			if strings.TrimSpace(label) == "" {
				continue
			}
			value := ""
			if i < len(row) {
				value = row[i]
			}
			r.pushLine(ctx.quotePrefix + markdownTableHeaderStyle().Render(label))
			for _, line := range wrapPlainLinePreserveWhitespace(value, valueWidth) {
				r.pushLine(ctx.quotePrefix + "  " + line)
			}
		}
		if rowIndex < len(rows)-1 {
			r.pushLine(ctx.quotePrefix + strings.Repeat("─", max(min(r.width, 80), 10)))
		}
	}
	r.ensureBlankAfterBlock()
}

func tableColumnWidths(header []string, rows [][]string) []int {
	widths := make([]int, len(header))
	for i, cell := range header {
		widths[i] = max(widths[i], lipgloss.Width(xansi.Strip(cell)))
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				continue
			}
			widths[i] = max(widths[i], min(lipgloss.Width(xansi.Strip(cell)), 38))
		}
	}
	for i := range widths {
		widths[i] = max(widths[i], 3)
	}
	return widths
}

func renderTableRow(row []string, widths []int, style lipgloss.Style) string {
	cells := make([]string, len(widths))
	for i, width := range widths {
		value := ""
		if i < len(row) {
			value = row[i]
		}
		cells[i] = padANSI(style.Render(value), width)
	}
	return strings.Join(cells, "  ")
}

func renderWrappedTableRow(row []string, widths []int) []string {
	wrapped := make([][]string, len(widths))
	height := 1
	for i, width := range widths {
		value := ""
		if i < len(row) {
			value = row[i]
		}
		lines := wrapPlainLinePreserveWhitespace(value, width)
		if len(lines) == 0 {
			lines = []string{""}
		}
		wrapped[i] = lines
		height = max(height, len(lines))
	}
	out := make([]string, 0, height)
	for line := 0; line < height; line++ {
		cells := make([]string, len(widths))
		for i, width := range widths {
			value := ""
			if line < len(wrapped[i]) {
				value = wrapped[i][line]
			}
			cells[i] = padANSI(value, width)
		}
		out = append(out, strings.Join(cells, "  "))
	}
	return out
}

func renderTableSeparator(widths []int, glyph string) string {
	parts := make([]string, len(widths))
	for i, width := range widths {
		parts[i] = markdownMutedStyle().Render(strings.Repeat(glyph, width))
	}
	return strings.Join(parts, "  ")
}

func padANSI(value string, width int) string {
	padding := width - lipgloss.Width(value)
	if padding < 0 {
		padding = 0
	}
	return value + strings.Repeat(" ", padding)
}

func wrapANSIText(content string, width int) []string {
	var out []string
	for _, rawLine := range strings.Split(content, "\n") {
		rawLine = strings.TrimSpace(rawLine)
		if rawLine == "" {
			out = append(out, "")
			continue
		}
		if lipgloss.Width(rawLine) <= width {
			out = append(out, rawLine)
			continue
		}
		words := strings.Fields(rawLine)
		current := ""
		for _, word := range words {
			if current == "" {
				current = word
				continue
			}
			next := current + " " + word
			if lipgloss.Width(next) <= width {
				current = next
				continue
			}
			out = append(out, current)
			current = word
		}
		if current != "" {
			out = append(out, current)
		}
	}
	return out
}

func markdownHeadingStyle(level int) lipgloss.Style {
	style := markdownStyleRenderer.NewStyle().Foreground(cAccentHi)
	switch level {
	case 1:
		return style.Bold(true).Underline(true)
	case 2:
		return style.Bold(true)
	case 3:
		return style.Bold(true).Italic(true)
	default:
		return style.Italic(true)
	}
}

func markdownInlineCodeStyle() lipgloss.Style {
	return markdownStyleRenderer.NewStyle().Foreground(cAccentHi)
}

func markdownCodeBlockStyle() lipgloss.Style {
	return markdownStyleRenderer.NewStyle().Foreground(cAccentHi)
}

func markdownLinkStyle() lipgloss.Style {
	return markdownStyleRenderer.NewStyle().Foreground(cAccentHi).Underline(true)
}

func markdownTableHeaderStyle() lipgloss.Style {
	return markdownStyleRenderer.NewStyle().Bold(true).Foreground(cAccentHi)
}

func markdownMutedStyle() lipgloss.Style {
	return markdownStyleRenderer.NewStyle().Foreground(cTextMuted)
}

func (r *codexMarkdownRenderer) localLinkDisplay(dest string) (string, bool) {
	target := strings.TrimSpace(dest)
	if target == "" || isWebLink(target) {
		return "", false
	}
	if strings.HasPrefix(target, "file://") {
		target = strings.TrimPrefix(target, "file://")
	}
	target = strings.TrimPrefix(target, "file:")
	location := ""
	if match := localLinkLocationSuffixRE.FindString(target); match != "" {
		location = match
		target = strings.TrimSuffix(target, match)
	}
	target = strings.ReplaceAll(target, "%20", " ")
	if strings.HasPrefix(target, "#") {
		return "", false
	}
	display := filepath.Clean(target)
	if filepath.IsAbs(target) && r.cwd != "" {
		if rel, err := filepath.Rel(r.cwd, display); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			display = rel
		}
	}
	if display == "." {
		return "", false
	}
	display = filepath.ToSlash(display) + location
	return display, true
}

func isWebLink(dest string) bool {
	lower := strings.ToLower(dest)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:")
}

func currentMarkdownCWD() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

func unwrapMarkdownTableFences(input string) string {
	lines := strings.SplitAfter(input, "\n")
	var out bytes.Buffer
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSuffix(lines[i], "\n")
		match := markdownFenceOpenRE.FindStringSubmatch(line)
		if len(match) == 0 || !isMarkdownFenceInfo(match[2]) {
			out.WriteString(lines[i])
			continue
		}
		marker := match[1]
		var body []string
		closed := false
		for j := i + 1; j < len(lines); j++ {
			candidate := strings.TrimSpace(strings.TrimSuffix(lines[j], "\n"))
			if strings.HasPrefix(candidate, marker[:1]) && len(candidate) >= len(marker) && strings.Trim(candidate, marker[:1]) == "" {
				i = j
				closed = true
				break
			}
			body = append(body, lines[j])
		}
		content := strings.Join(body, "")
		if closed && containsMarkdownTable(content) {
			out.WriteString(content)
			if !strings.HasSuffix(content, "\n") {
				out.WriteString("\n")
			}
			continue
		}
		out.WriteString(lines[i])
	}
	return out.String()
}

func isMarkdownFenceInfo(info string) bool {
	info = strings.ToLower(strings.TrimSpace(info))
	return info == "md" || info == "markdown"
}

func containsMarkdownTable(content string) bool {
	var previous string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			previous = ""
			continue
		}
		if previous != "" && looksLikeTableHeader(previous) && looksLikeTableDelimiter(trimmed) {
			return true
		}
		previous = trimmed
	}
	return false
}

func looksLikeTableHeader(line string) bool {
	return strings.Contains(line, "|") && !looksLikeTableDelimiter(line)
}

func looksLikeTableDelimiter(line string) bool {
	trimmed := strings.Trim(line, "| ")
	if trimmed == "" {
		return false
	}
	for _, part := range strings.Split(trimmed, "|") {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, ":")
		if len(part) < 3 || strings.Trim(part, "-") != "" {
			return false
		}
	}
	return true
}
