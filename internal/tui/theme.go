package tui

import (
	"sort"
	"strings"

	"github.com/Zts0hg/foxharness/internal/tui/selector"
	"github.com/charmbracelet/lipgloss"
)

type tuiTheme struct {
	name          string
	bg            string
	panel         string
	accent        string
	accentHi      string
	warn          string
	textPri       string
	textSec       string
	textMuted     string
	textDim       string
	divider       string
	progressEmpty string
	selectionBg   string
	selectionFg   string
}

var builtInThemes = map[string]tuiTheme{
	"codex": {
		name:          "codex",
		bg:            "#0b0d10",
		panel:         "#15181d",
		accent:        "#8ec5ff",
		accentHi:      "#d7e8ff",
		warn:          "#ffb86b",
		textPri:       "#e6edf3",
		textSec:       "#c9d1d9",
		textMuted:     "#8b949e",
		textDim:       "#6e7681",
		divider:       "#30363d",
		progressEmpty: "#30363d",
		selectionBg:   "#264f78",
		selectionFg:   "#ffffff",
	},
	"amber": {
		name:          "amber",
		bg:            amberBgHex,
		panel:         amberPanelHex,
		accent:        amberHex,
		accentHi:      amberHiHex,
		warn:          amberWarnHex,
		textPri:       amberHiHex,
		textSec:       amberHex,
		textMuted:     amberMutedHex,
		textDim:       amberDimHex,
		divider:       amberDividerHex,
		progressEmpty: amberProgressEmptyHex,
		selectionBg:   selectionBgHex,
		selectionFg:   selectionFgHex,
	},
	"mono": {
		name:          "mono",
		bg:            "#0d0d0d",
		panel:         "#1a1a1a",
		accent:        "#d4d4d4",
		accentHi:      "#ffffff",
		warn:          "#bdbdbd",
		textPri:       "#f5f5f5",
		textSec:       "#d4d4d4",
		textMuted:     "#a3a3a3",
		textDim:       "#737373",
		divider:       "#404040",
		progressEmpty: "#2a2a2a",
		selectionBg:   "#e5e5e5",
		selectionFg:   "#111111",
	},
	"light": {
		name:          "light",
		bg:            "#fbfbfa",
		panel:         "#f0f1f2",
		accent:        "#005f87",
		accentHi:      "#003f5c",
		warn:          "#9a3412",
		textPri:       "#1f2933",
		textSec:       "#334155",
		textMuted:     "#64748b",
		textDim:       "#94a3b8",
		divider:       "#cbd5e1",
		progressEmpty: "#dbe3ea",
		selectionBg:   "#bfdbfe",
		selectionFg:   "#111827",
	},
}

func normalizeThemeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func isBuiltInTheme(name string) bool {
	_, ok := builtInThemes[normalizeThemeName(name)]
	return ok
}

func builtInThemeNames() []string {
	names := make([]string, 0, len(builtInThemes))
	for name := range builtInThemes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func applyTheme(name string) string {
	theme, ok := builtInThemes[normalizeThemeName(name)]
	if !ok {
		theme = builtInThemes[defaultThemeName]
	}
	cBg = lipgloss.Color(theme.bg)
	cAccent = lipgloss.Color(theme.accent)
	cAccentHi = lipgloss.Color(theme.accentHi)
	cWarn = lipgloss.Color(theme.warn)
	cTextPri = lipgloss.Color(theme.textPri)
	cTextSec = lipgloss.Color(theme.textSec)
	cTextMuted = lipgloss.Color(theme.textMuted)
	cTextDim = lipgloss.Color(theme.textDim)
	cTextVeryDim = lipgloss.Color(theme.divider)
	cMsgBg = lipgloss.Color(theme.panel)
	cProgressEmpty = lipgloss.Color(theme.progressEmpty)
	cSelectionBg = lipgloss.Color(theme.selectionBg)
	cSelectionFg = lipgloss.Color(theme.selectionFg)
	rebuildStyles()
	selector.ApplyPalette(selector.Palette{
		Title:    cAccentHi,
		Cursor:   cWarn,
		Muted:    cTextMuted,
		Selected: cAccent,
	})
	applyMarkdownTheme(theme)
	return theme.name
}

func rebuildStyles() {
	outerStyle = lipgloss.NewStyle().
		Foreground(cAccent).
		Padding(viewPaddingTop, viewPaddingRight, viewPaddingBottom, viewPaddingLeft)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	headerMetaStyle = lipgloss.NewStyle().Foreground(cTextDim)
	bodyStyle = lipgloss.NewStyle().Foreground(cTextSec)
	inputStyle = lipgloss.NewStyle().
		Foreground(cTextSec).
		Border(lipgloss.Border{Top: "─", Bottom: "─"}, true, false, true, false).
		BorderForeground(cTextVeryDim)
	runningNoticeStyle = lipgloss.NewStyle().Foreground(cWorkingText)
	workingGlyphStyle = lipgloss.NewStyle().Bold(true).Foreground(cWorkingText)
	workingTextStyle = lipgloss.NewStyle().Foreground(cWorkingText)
	workingShimmerStyle = lipgloss.NewStyle().Bold(true).Foreground(cWorkingShimmer)
	suggestionStyle = lipgloss.NewStyle().
		Foreground(cTextSec).
		Border(lipgloss.Border{Left: "┊"}, false, false, false, true).
		BorderForeground(cTextVeryDim).
		Padding(0, 1)
	suggestionCommandStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentHi)
	suggestionSelectedStyle = lipgloss.NewStyle().Foreground(cWarn)
	suggestionDescriptionStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	footerStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	selectionStyle = lipgloss.NewStyle().Foreground(cSelectionFg).Background(cSelectionBg)
	userBubbleStyle = lipgloss.NewStyle().
		Foreground(cAccent).
		Background(cMsgBg).
		Border(lipgloss.Border{Left: "▌"}, false, false, false, true).
		BorderForeground(cAccent).
		BorderBackground(cMsgBg).
		Padding(0, 1)
	userMetaStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentHi)
	assistantLabelStyle = lipgloss.NewStyle().Foreground(cAccentHi)
	toolLabelStyle = lipgloss.NewStyle().Foreground(cWarn)
	systemLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(cTextSec)
	errorLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(cWarn)
	commandLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	mutedStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	placeholderStyle = lipgloss.NewStyle().Foreground(cTextDim).Italic(true)
	cursorStyle = lipgloss.NewStyle().Foreground(cAccentHi)
	hintStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	planModeStyle = lipgloss.NewStyle().Foreground(cWarn)
	statusModelStyle = lipgloss.NewStyle().Foreground(cAccentHi)
	statusProjectStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	statusGitStyle = lipgloss.NewStyle().Foreground(cWarn)
	statusDimStyle = lipgloss.NewStyle().Foreground(cTextVeryDim)
	statusFaintStyle = lipgloss.NewStyle().Foreground(cTextDim)
	contextLowStyle = lipgloss.NewStyle().Foreground(cAccentHi)
	contextMediumStyle = lipgloss.NewStyle().Foreground(cWarn)
	contextHighStyle = lipgloss.NewStyle().Foreground(cWarn)
	sidebarBoxStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	sidebarFocusedBoxStyle = lipgloss.NewStyle().Foreground(cTextSec)
	sidebarTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	sidebarFocusedTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(cWarn)
	sidebarDividerStyle = lipgloss.NewStyle().Foreground(cTextVeryDim)
	askFocusedStyle = lipgloss.NewStyle().Foreground(cAccentHi)
}
