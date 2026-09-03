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
	// codeHighlight names the chroma style used for fenced code blocks. An
	// empty value leaves the choice to the detected terminal background, which
	// is what terminal-following and background-agnostic themes want.
	codeHighlight string
	// quote and listMarker color the two markdown elements that carry their own
	// hue rather than the accent. A theme with a fixed palette names them so
	// they stay inside the scheme; an empty value keeps the ANSI index, letting
	// a terminal-following theme use the terminal's own green and blue.
	quote      string
	listMarker string
	// followsTerminal marks a theme that inherits the terminal's own colors
	// (empty foreground/background plus ANSI-indexed accents) and should be
	// adapted to the detected terminal background, the way codex does.
	followsTerminal bool
}

var builtInThemes = map[string]tuiTheme{
	// monokai-pro is the default. It maps Fox's roles onto the published
	// Monokai Pro filter: blue and purple take the accent slots the codex theme
	// fills with ANSI cyan and magenta, red carries warnings, and the dimmed
	// scale supplies the text hierarchy from primary down to dividers. The
	// markdown quote and list-marker colors follow monokaiDarkPalette, which
	// already renders their ANSI indices (2 and 12) as this green and orange
	// when a frame is exported, so terminal and export agree.
	"monokai-pro": {
		name:          "monokai-pro",
		bg:            monokaiBackground,
		panel:         monokaiDark1,
		accent:        monokaiBlue,
		accentHi:      monokaiPurple,
		warn:          monokaiRed,
		textPri:       monokaiText,
		textSec:       monokaiDimmed1,
		textMuted:     monokaiDimmed2,
		textDim:       monokaiDimmed3,
		divider:       monokaiDimmed4,
		progressEmpty: monokaiDimmed5,
		selectionBg:   monokaiBlue,
		selectionFg:   monokaiBackground,
		codeHighlight: monokaiChromaStyle,
		quote:         monokaiGreen,
		listMarker:    monokaiOrange,
	},
	"codex": {
		name:            "codex",
		bg:              "",
		panel:           "",
		accent:          "6",
		accentHi:        "5",
		warn:            "1",
		textPri:         "",
		textSec:         "",
		textMuted:       "8",
		textDim:         "8",
		divider:         "8",
		progressEmpty:   "8",
		selectionBg:     "6",
		selectionFg:     "",
		followsTerminal: true,
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
	bg, known := terminalBackground()
	theme = adaptThemeToTerminal(theme, bg, known)
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
	applyMarkdownTheme(theme, known && bg.isLight())
	return theme.name
}

func rebuildStyles() {
	outerStyle = lipgloss.NewStyle().
		Foreground(cTextPri).
		Padding(viewPaddingTop, viewPaddingRight, viewPaddingBottom, viewPaddingLeft)
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	headerMetaStyle = lipgloss.NewStyle().Foreground(cTextDim)
	bodyStyle = lipgloss.NewStyle().Foreground(cTextSec)
	inputStyle = lipgloss.NewStyle().
		Foreground(cTextSec).
		Border(lipgloss.Border{Top: "─", Bottom: "─"}, true, false, true, false).
		BorderForeground(cTextVeryDim)
	runningNoticeStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	workingGlyphStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	workingTextStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	workingShimmerStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	suggestionStyle = lipgloss.NewStyle().
		Foreground(cTextSec).
		Border(lipgloss.Border{Left: "┊"}, false, false, false, true).
		BorderForeground(cTextVeryDim).
		Padding(0, 1)
	suggestionCommandStyle = lipgloss.NewStyle().Foreground(cTextPri)
	suggestionSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	suggestionDescriptionStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	footerStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	selectionStyle = lipgloss.NewStyle().Foreground(cSelectionFg).Background(cSelectionBg)
	userBubbleStyle = lipgloss.NewStyle().Foreground(cTextPri)
	userMetaStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccentHi)
	assistantLabelStyle = lipgloss.NewStyle().Foreground(cTextPri)
	toolLabelStyle = lipgloss.NewStyle().Foreground(cTextMuted)
	systemLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(cTextSec)
	errorLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(cWarn)
	commandLabelStyle = lipgloss.NewStyle().Bold(true).Foreground(cAccent)
	userBarStyle = lipgloss.NewStyle().Foreground(cAccent)
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
