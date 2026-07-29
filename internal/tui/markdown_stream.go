package tui

import "strings"

type markdownStreamController struct {
	width  int
	source strings.Builder
	tail   string
}

func newMarkdownStreamController(width int) *markdownStreamController {
	return &markdownStreamController{width: max(width, 20)}
}

func (s *markdownStreamController) Push(delta string) string {
	if s == nil || delta == "" {
		return ""
	}
	s.tail += delta
	lastNewline := strings.LastIndex(s.tail, "\n")
	if lastNewline < 0 {
		return s.renderCommitted()
	}
	s.source.WriteString(s.tail[:lastNewline+1])
	s.tail = s.tail[lastNewline+1:]
	return s.renderCommitted()
}

func (s *markdownStreamController) Finish() string {
	if s == nil {
		return ""
	}
	if s.tail != "" {
		s.source.WriteString(s.tail)
		s.tail = ""
	}
	return s.renderCommitted()
}

func (s *markdownStreamController) renderCommitted() string {
	if s == nil || s.source.Len() == 0 {
		return ""
	}
	body := renderMarkdown(s.source.String(), max(s.width-2, 20))
	if body == "" {
		return ""
	}
	return renderCodexPrefixedCell(body, "• ", "  ")
}
