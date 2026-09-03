package tui

import "testing"

func TestMonokaiProThemeUsesOfficialPalette(t *testing.T) {
	theme, ok := builtInThemes["monokai-pro"]
	if !ok {
		t.Fatalf(`builtInThemes has no "monokai-pro" entry; available: %v`, builtInThemeNames())
	}
	if theme.name != "monokai-pro" {
		t.Fatalf("theme name = %q, want monokai-pro", theme.name)
	}
	if theme.followsTerminal {
		t.Fatal("monokai-pro must be a fixed palette, not a terminal-following theme")
	}
	fields := []struct {
		field string
		got   string
		want  string
	}{
		{"bg", theme.bg, "#2d2a2e"},
		{"panel", theme.panel, "#221f22"},
		{"accent", theme.accent, "#78dce8"},
		{"accentHi", theme.accentHi, "#ab9df2"},
		{"warn", theme.warn, "#ff6188"},
		{"textPri", theme.textPri, "#fcfcfa"},
		{"textSec", theme.textSec, "#c1c0c0"},
		{"textMuted", theme.textMuted, "#939293"},
		{"textDim", theme.textDim, "#727072"},
		{"divider", theme.divider, "#5b595c"},
		{"progressEmpty", theme.progressEmpty, "#403e41"},
		{"selectionBg", theme.selectionBg, "#78dce8"},
		{"selectionFg", theme.selectionFg, "#2d2a2e"},
		{"codeHighlight", theme.codeHighlight, "monokai"},
	}
	for _, f := range fields {
		if f.got != f.want {
			t.Errorf("monokai-pro %s = %q, want %q", f.field, f.got, f.want)
		}
	}
}

func TestDefaultThemeIsMonokaiPro(t *testing.T) {
	if defaultThemeName != "monokai-pro" {
		t.Fatalf("defaultThemeName = %q, want monokai-pro", defaultThemeName)
	}
	if !isBuiltInTheme(defaultThemeName) {
		t.Fatalf("default theme %q is not registered as a built-in", defaultThemeName)
	}

	t.Cleanup(func() { applyTheme(defaultThemeName) })
	if got := applyTheme("no-such-theme"); got != "monokai-pro" {
		t.Fatalf("applyTheme fallback = %q, want monokai-pro", got)
	}
}

func TestApplyMonokaiProThemeIgnoresLightTerminalAndSelectsMonokaiHighlighting(t *testing.T) {
	origProbe := terminalBackgroundProbe
	t.Cleanup(func() {
		resetTerminalBackgroundCache(t, origProbe)
		applyTheme(defaultThemeName)
	})
	resetTerminalBackgroundCache(t, func() (rgb, bool) { return rgb{250, 250, 250}, true })

	if got := applyTheme("monokai-pro"); got != "monokai-pro" {
		t.Fatalf("applyTheme(monokai-pro) = %q, want monokai-pro", got)
	}
	if string(cAccent) != "#78dce8" {
		t.Fatalf("accent = %q, want the fixed Monokai Pro blue #78dce8", string(cAccent))
	}
	if string(cTextPri) != "#fcfcfa" {
		t.Fatalf("primary text = %q, want #fcfcfa", string(cTextPri))
	}
	if markdownCodeHighlightStyle != "monokai" {
		t.Fatalf("code highlight style = %q, want monokai", markdownCodeHighlightStyle)
	}
	if string(markdownAccentColor) != "#78dce8" {
		t.Fatalf("markdown accent = %q, want #78dce8", string(markdownAccentColor))
	}
}

func TestThemesWithoutCodeHighlightKeepBackgroundDerivedStyle(t *testing.T) {
	origProbe := terminalBackgroundProbe
	t.Cleanup(func() {
		resetTerminalBackgroundCache(t, origProbe)
		applyTheme(defaultThemeName)
	})
	resetTerminalBackgroundCache(t, func() (rgb, bool) { return rgb{18, 18, 18}, true })

	applyTheme("amber")
	if markdownCodeHighlightStyle != "github-dark" {
		t.Fatalf("amber code highlight style = %q, want github-dark", markdownCodeHighlightStyle)
	}
}
