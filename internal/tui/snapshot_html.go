package tui

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/Zts0hg/foxharness/internal/app"
)

// sarasaWOFF2 is Sarasa Mono SC (OFL, see assets/README.md), embedded so HTML
// exports are self-contained and render CJK, box-drawing, and symbol glyphs
// with correct monospace metrics in any browser.
//
//go:embed assets/SarasaMonoSC.woff2
var sarasaWOFF2 []byte

// RenderSceneHTML renders the named scene at the given terminal size to a
// self-contained HTML document. The document embeds the Monokai Pro palette and
// the Sarasa Mono SC font, so a browser reproduces the frame faithfully —
// including glyphs a single Latin font would drop. Unknown scene names return an
// error listing the available scenes.
func RenderSceneHTML(name string, width, height int) (string, error) {
	frame, err := RenderSceneANSI(name, width, height)
	if err != nil {
		return "", err
	}
	return frameToHTML(RecolorForImage(frame)), nil
}

// RenderSessionHTML renders a real session's message records into the same
// self-contained HTML document, so an actual conversation can be inspected the
// way a browser would show it.
func RenderSessionHTML(records []app.ConversationRecord, width, height int) string {
	return frameToHTML(RecolorForImage(RenderSessionANSI(records, width, height)))
}

// frameToHTML wraps an ANSI frame (already recolored to truecolor) in a
// self-contained HTML document with the embedded font and background.
func frameToHTML(frame string) string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><style>`)
	b.WriteString(`@font-face{font-family:"SarasaMonoSC";font-display:block;src:url("data:font/woff2;base64,`)
	b.WriteString(base64.StdEncoding.EncodeToString(sarasaWOFF2))
	b.WriteString(`") format("woff2");}`)
	b.WriteString(`html,body{margin:0;background:`)
	b.WriteString(SnapshotBackground)
	b.WriteString(`;}pre{font-family:"SarasaMonoSC",monospace;font-size:16px;line-height:1.25;color:`)
	b.WriteString(snapshotForeground)
	b.WriteString(`;padding:16px;margin:0;white-space:pre;display:inline-block;}</style></head><body><pre>`)
	b.WriteString(ansiToSpans(frame))
	b.WriteString(`</pre></body></html>`)
	return b.String()
}

// spanState is the active SGR styling while converting an ANSI frame to HTML.
type spanState struct {
	fg        string
	bg        string
	bold      bool
	faint     bool
	underline bool
	reverse   bool
}

func (s spanState) empty() bool {
	return s == spanState{}
}

// css renders the current state as inline CSS, resolving reverse-video against
// the document's default foreground and background.
func (s spanState) css() string {
	fg, bg := s.fg, s.bg
	if s.reverse {
		fg, bg = bg, fg
		if fg == "" {
			fg = SnapshotBackground
		}
		if bg == "" {
			bg = snapshotForeground
		}
	}
	var parts []string
	if fg != "" {
		parts = append(parts, "color:"+fg)
	}
	if bg != "" {
		parts = append(parts, "background:"+bg)
	}
	if s.bold {
		parts = append(parts, "font-weight:700")
	}
	if s.faint {
		parts = append(parts, "opacity:.55")
	}
	if s.underline {
		parts = append(parts, "text-decoration:underline")
	}
	return strings.Join(parts, ";")
}

// ansiToSpans converts an ANSI (truecolor) frame into HTML span markup. Only the
// SGR subset lipgloss emits is handled; unknown parameters are ignored.
func ansiToSpans(frame string) string {
	var b strings.Builder
	var st spanState
	var text strings.Builder

	flush := func() {
		if text.Len() == 0 {
			return
		}
		content := htmlEscape(text.String())
		if style := st.css(); style != "" {
			b.WriteString(`<span style="`)
			b.WriteString(style)
			b.WriteString(`">`)
			b.WriteString(content)
			b.WriteString(`</span>`)
		} else {
			b.WriteString(content)
		}
		text.Reset()
	}

	i := 0
	for i < len(frame) {
		if frame[i] == 0x1b && i+1 < len(frame) && frame[i+1] == '[' {
			end := strings.IndexByte(frame[i:], 'm')
			if end > 0 {
				flush()
				applySGR(&st, frame[i+2:i+end])
				i += end + 1
				continue
			}
		}
		text.WriteByte(frame[i])
		i++
	}
	flush()
	return b.String()
}

// applySGR mutates the span state per one SGR parameter list.
func applySGR(st *spanState, params string) {
	fields := strings.Split(params, ";")
	for k := 0; k < len(fields); k++ {
		n, err := strconv.Atoi(fields[k])
		if err != nil {
			continue
		}
		switch {
		case n == 0:
			*st = spanState{}
		case n == 1:
			st.bold = true
		case n == 2:
			st.faint = true
		case n == 4:
			st.underline = true
		case n == 7:
			st.reverse = true
		case n == 22:
			st.bold, st.faint = false, false
		case n == 24:
			st.underline = false
		case n == 27:
			st.reverse = false
		case n == 39:
			st.fg = ""
		case n == 49:
			st.bg = ""
		case n == 38 && k+4 < len(fields) && fields[k+1] == "2":
			st.fg = rgbCSS(fields[k+2], fields[k+3], fields[k+4])
			k += 4
		case n == 48 && k+4 < len(fields) && fields[k+1] == "2":
			st.bg = rgbCSS(fields[k+2], fields[k+3], fields[k+4])
			k += 4
		}
	}
}

func rgbCSS(r, g, b string) string {
	return fmt.Sprintf("rgb(%s,%s,%s)", r, g, b)
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
