package tui

import (
	"regexp"
	"strings"

	xansi "github.com/charmbracelet/x/ansi"
)

const osc8Terminator = "\x1b\\"

var oscEscapePattern = regexp.MustCompile(`\x1b\][^\x1b]*(?:\x1b\\|\x07)`)

type terminalHyperlink struct {
	start int
	end   int
	url   string
}

type lineWithHyperlinks struct {
	text  string
	links []terminalHyperlink
}

func (l lineWithHyperlinks) render() string {
	if len(l.links) == 0 {
		return l.text
	}
	var out strings.Builder
	cursor := 0
	for _, link := range l.links {
		if link.start < cursor || link.end > len(l.text) || link.start >= link.end || !isClickableWebLink(link.url) {
			continue
		}
		out.WriteString(l.text[cursor:link.start])
		out.WriteString(renderTerminalHyperlink(link.url, l.text[link.start:link.end]))
		cursor = link.end
	}
	out.WriteString(l.text[cursor:])
	return out.String()
}

func renderTerminalHyperlink(url string, label string) string {
	if !isClickableWebLink(url) || label == "" {
		return label
	}
	return "\x1b]8;;" + url + osc8Terminator + label + "\x1b]8;;" + osc8Terminator
}

func isClickableWebLink(dest string) bool {
	lower := strings.ToLower(strings.TrimSpace(dest))
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}

func stripTerminalControl(s string) string {
	return xansi.Strip(oscEscapePattern.ReplaceAllString(s, ""))
}
