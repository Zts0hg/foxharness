package tui

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
)

var markdownRenderers sync.Map

var tuiMarkdownStyle = ansi.StyleConfig{
	BlockQuote: ansi.StyleBlock{
		Indent:      uintPtr(1),
		IndentToken: stringPtr("│ "),
	},
	List: ansi.StyleList{
		LevelIndent: 2,
	},
	Heading: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Bold:        boolPtr(true),
			Color:       stringPtr("114"),
			BlockSuffix: "\n",
		},
	},
	H1: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "# "}},
	H2: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "## "}},
	H3: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "### "}},
	H4: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "#### "}},
	H5: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "##### "}},
	H6: ansi.StyleBlock{StylePrimitive: ansi.StylePrimitive{Prefix: "###### "}},
	Strong: ansi.StylePrimitive{
		Bold: boolPtr(true),
	},
	Emph: ansi.StylePrimitive{
		Italic: boolPtr(true),
	},
	Strikethrough: ansi.StylePrimitive{
		CrossedOut: boolPtr(true),
	},
	HorizontalRule: ansi.StylePrimitive{
		Color:  stringPtr("240"),
		Format: "\n--------\n",
	},
	Item: ansi.StylePrimitive{
		BlockPrefix: "• ",
	},
	Enumeration: ansi.StylePrimitive{
		BlockPrefix: ". ",
	},
	Task: ansi.StyleTask{
		Ticked:   "[✓] ",
		Unticked: "[ ] ",
	},
	Link: ansi.StylePrimitive{
		Color:     stringPtr("81"),
		Underline: boolPtr(true),
	},
	LinkText: ansi.StylePrimitive{
		Color: stringPtr("81"),
		Bold:  boolPtr(true),
	},
	Code: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix:          " ",
			Suffix:          " ",
			Color:           stringPtr("203"),
			BackgroundColor: stringPtr("236"),
		},
	},
	CodeBlock: ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("252"),
			},
			Indent:      uintPtr(1),
			IndentToken: stringPtr("  "),
		},
	},
	Table: ansi.StyleTable{
		CenterSeparator: stringPtr("|"),
		ColumnSeparator: stringPtr("|"),
		RowSeparator:    stringPtr("-"),
	},
}

func renderMarkdown(text string, width int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	width = max(width, 20)

	renderer, err := markdownRenderer(width)
	if err != nil {
		return wrapText(text, width)
	}
	out, err := renderer.Render(text)
	if err != nil {
		return wrapText(text, width)
	}
	out = strings.Trim(out, "\n")
	if strings.TrimSpace(out) == "" {
		return wrapText(text, width)
	}
	return out
}

func markdownRenderer(width int) (*glamour.TermRenderer, error) {
	if renderer, ok := markdownRenderers.Load(width); ok {
		return renderer.(*glamour.TermRenderer), nil
	}
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(tuiMarkdownStyle),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	actual, _ := markdownRenderers.LoadOrStore(width, renderer)
	return actual.(*glamour.TermRenderer), nil
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func uintPtr(value uint) *uint {
	return &value
}
