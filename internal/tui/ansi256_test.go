package tui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestNearestANSI256KeepsTheDimmedScaleDistinct(t *testing.T) {
	// termenv's own conversion forces near-neutral colors into the 6x6x6 cube,
	// whose dark band is a single bucket from 48 to 114. That puts these five
	// steps of the Monokai Pro dimmed scale on one index, so dividers, dim
	// text, and the progress track become indistinguishable.
	scale := []struct {
		role string
		hex  string
	}{
		{"textSec", monokaiDimmed1},
		{"textMuted", monokaiDimmed2},
		{"textDim", monokaiDimmed3},
		{"divider", monokaiDimmed4},
		{"progressEmpty", monokaiDimmed5},
	}
	seen := map[string]string{}
	for _, s := range scale {
		index, ok := nearestANSI256(s.hex)
		if !ok {
			t.Fatalf("%s: nearestANSI256(%q) reported no conversion", s.role, s.hex)
		}
		if previous, clash := seen[index]; clash {
			t.Fatalf("%s and %s both map to palette index %s", previous, s.role, index)
		}
		seen[index] = s.role
	}
}

func TestNearestANSI256PicksTheClosestFixedPaletteEntry(t *testing.T) {
	// Indices 0-15 are whatever the user configured in their terminal, so a
	// fixed theme must never resolve to one; the search starts at 16.
	tests := []struct {
		hex   string
		index string
		entry string
	}{
		{monokaiText, "231", "#ffffff"},
		{monokaiDimmed1, "250", "#bcbcbc"},
		{monokaiDimmed2, "246", "#949494"},
		{monokaiDimmed3, "242", "#6c6c6c"},
		{monokaiDimmed4, "240", "#585858"},
		{monokaiDimmed5, "237", "#3a3a3a"},
		{monokaiDark1, "234", "#1c1c1c"},
		{monokaiBlue, "116", "#87d7d7"},
		{monokaiPurple, "147", "#afafff"},
		{monokaiRed, "204", "#ff5f87"},
	}
	for _, tt := range tests {
		index, ok := nearestANSI256(tt.hex)
		if !ok {
			t.Fatalf("nearestANSI256(%q) reported no conversion", tt.hex)
		}
		if index != tt.index {
			t.Errorf("nearestANSI256(%q) = %s, want %s (%s)", tt.hex, index, tt.index, tt.entry)
		}
	}
}

func TestNearestANSI256IgnoresValuesThatAreNotTruecolor(t *testing.T) {
	// Terminal-following themes carry ANSI indices and empty strings, which
	// already mean "let the terminal decide" and must pass through untouched.
	for _, value := range []string{"", "6", "8", "not-a-color", "#12345"} {
		if index, ok := nearestANSI256(value); ok {
			t.Errorf("nearestANSI256(%q) converted to %q, want no conversion", value, index)
		}
	}
}

func TestApplyThemeResolvesPaletteIndicesOnA256ColorTerminal(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
		applyTheme(defaultThemeName)
	})
	lipgloss.SetColorProfile(termenv.ANSI256)
	applyTheme("monokai-pro")

	if string(cTextDim) == string(cTextVeryDim) {
		t.Fatalf("dim text and dividers collapsed onto %q", string(cTextDim))
	}
	for _, c := range []struct {
		role  string
		value string
	}{
		{"textDim", string(cTextDim)},
		{"divider", string(cTextVeryDim)},
		{"progressEmpty", string(cProgressEmpty)},
	} {
		if c.value == "" || c.value[0] == '#' {
			t.Errorf("%s = %q, want a palette index on a 256-color terminal", c.role, c.value)
		}
	}
}

func TestApplyThemeKeepsTruecolorWhenTheTerminalSupportsIt(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() {
		lipgloss.SetColorProfile(prev)
		applyTheme(defaultThemeName)
	})
	for _, profile := range []termenv.Profile{termenv.TrueColor, termenv.Ascii} {
		lipgloss.SetColorProfile(profile)
		applyTheme("monokai-pro")
		if got := string(cTextDim); got != monokaiDimmed3 {
			t.Errorf("profile %v: textDim = %q, want the published %q", profile, got, monokaiDimmed3)
		}
	}
}
