package tui

import (
	"os"
	"sync"

	"github.com/muesli/termenv"
	"golang.org/x/term"
)

// lightBgAccentHex is the accent color codex substitutes for cyan on light
// terminal backgrounds (style.rs LIGHT_BG_ACCENT_RGB = (0, 95, 135)). Plain
// ANSI cyan washes out on light themes, so a darker teal keeps active and
// selected controls legible.
const lightBgAccentHex = "#005f87"

// rgb is an 8-bit-per-channel color used for terminal-theme calculations.
type rgb struct {
	r, g, b uint8
}

// isLight reports whether a background color is perceptually light. It mirrors
// codex color.rs is_light: a Rec. 601 luma above the midpoint counts as light.
func (c rgb) isLight() bool {
	y := 0.299*float64(c.r) + 0.587*float64(c.g) + 0.114*float64(c.b)
	return y > 128.0
}

// terminalBackgroundProbe resolves the terminal's background color. It is a
// package variable so tests can inject a deterministic result instead of
// touching the real terminal.
var terminalBackgroundProbe = probeTerminalBackground

var (
	terminalBackgroundOnce  sync.Once
	terminalBackgroundColor rgb
	terminalBackgroundKnown bool
)

// terminalBackground returns the detected terminal background color and whether
// detection succeeded. The result is cached after the first call so the OSC
// query runs at most once per process, before the Bubble Tea event loop starts.
func terminalBackground() (rgb, bool) {
	terminalBackgroundOnce.Do(func() {
		terminalBackgroundColor, terminalBackgroundKnown = terminalBackgroundProbe()
	})
	return terminalBackgroundColor, terminalBackgroundKnown
}

// probeTerminalBackground queries the terminal for its background color via
// OSC 11 (through termenv). It stays silent when stdout is not a terminal so
// non-interactive runs and tests never emit escape sequences.
func probeTerminalBackground() (rgb, bool) {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return rgb{}, false
	}
	c := termenv.ConvertToRGB(termenv.BackgroundColor())
	return rgb{to255(c.R), to255(c.G), to255(c.B)}, true
}

// to255 maps a [0,1] color component to an 8-bit channel, rounding to nearest.
func to255(v float64) uint8 {
	n := v*255 + 0.5
	switch {
	case n < 0:
		return 0
	case n > 255:
		return 255
	default:
		return uint8(n)
	}
}

// adaptThemeToTerminal returns a copy of a terminal-following theme adjusted for
// the detected background. On a known light background it swaps codex's cyan
// accent for the darker teal codex uses (accent_style_for), keeping the
// dark/unknown defaults otherwise.
func adaptThemeToTerminal(theme tuiTheme, bg rgb, known bool) tuiTheme {
	if !theme.followsTerminal || !known || !bg.isLight() {
		return theme
	}
	theme.accent = lightBgAccentHex
	theme.accentHi = lightBgAccentHex
	theme.selectionBg = lightBgAccentHex
	return theme
}
