package tui

import (
	"sync"
	"testing"
)

func TestRGBIsLightMatchesCodexThreshold(t *testing.T) {
	tests := []struct {
		name string
		in   rgb
		want bool
	}{
		{name: "white", in: rgb{255, 255, 255}, want: true},
		{name: "black", in: rgb{0, 0, 0}, want: false},
		{name: "mid grey stays dark at threshold", in: rgb{128, 128, 128}, want: false},
		{name: "light grey", in: rgb{200, 200, 200}, want: true},
		{name: "typical dark terminal", in: rgb{30, 30, 30}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.isLight(); got != tt.want {
				t.Fatalf("isLight(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestAdaptThemeToTerminal(t *testing.T) {
	codex := builtInThemes["codex"]
	amber := builtInThemes["amber"]

	light := adaptThemeToTerminal(codex, rgb{250, 250, 250}, true)
	if light.accent != lightBgAccentHex {
		t.Fatalf("light accent = %q, want %q", light.accent, lightBgAccentHex)
	}

	dark := adaptThemeToTerminal(codex, rgb{20, 20, 20}, true)
	if dark.accent != "6" {
		t.Fatalf("dark accent = %q, want unchanged cyan 6", dark.accent)
	}

	unknown := adaptThemeToTerminal(codex, rgb{}, false)
	if unknown.accent != "6" {
		t.Fatalf("unknown accent = %q, want unchanged cyan 6", unknown.accent)
	}

	nonFollowing := adaptThemeToTerminal(amber, rgb{250, 250, 250}, true)
	if nonFollowing.accent != amber.accent {
		t.Fatalf("non-following theme accent changed to %q, want %q", nonFollowing.accent, amber.accent)
	}
}

func resetTerminalBackgroundCache(t *testing.T, probe func() (rgb, bool)) {
	t.Helper()
	terminalBackgroundProbe = probe
	terminalBackgroundOnce = sync.Once{}
	terminalBackgroundColor = rgb{}
	terminalBackgroundKnown = false
}

func TestApplyThemeFollowsTerminalBackground(t *testing.T) {
	origProbe := terminalBackgroundProbe
	t.Cleanup(func() {
		resetTerminalBackgroundCache(t, origProbe)
		applyTheme(defaultThemeName)
	})

	resetTerminalBackgroundCache(t, func() (rgb, bool) { return rgb{250, 250, 250}, true })
	applyTheme("codex")
	if string(cAccent) != lightBgAccentHex {
		t.Fatalf("light-terminal accent = %q, want %q", string(cAccent), lightBgAccentHex)
	}
	if markdownCodeHighlightStyle != "github" {
		t.Fatalf("light-terminal code style = %q, want github", markdownCodeHighlightStyle)
	}
	if string(markdownAccentColor) != lightBgAccentHex {
		t.Fatalf("light-terminal markdown accent = %q, want %q", string(markdownAccentColor), lightBgAccentHex)
	}

	resetTerminalBackgroundCache(t, func() (rgb, bool) { return rgb{18, 18, 18}, true })
	applyTheme("codex")
	if string(cAccent) != "6" {
		t.Fatalf("dark-terminal accent = %q, want 6", string(cAccent))
	}
	if markdownCodeHighlightStyle != "github-dark" {
		t.Fatalf("dark-terminal code style = %q, want github-dark", markdownCodeHighlightStyle)
	}
}
