package tui

/*
Monokai Pro palette, as published for the default "Monokai Pro" filter
(https://monokai.pro). These constants are the single authority for the
scheme's colors: the built-in TUI theme, the image/HTML export background and
foreground, and the ANSI recoloring table all derive from them rather than
repeating literals, so the terminal and an exported frame cannot drift apart.

The scheme is a fixed dark palette. Fox does not paint the terminal background,
so on a light terminal the `light` theme remains the legible choice.
*/
const (
	monokaiDark1      = "#221f22"
	monokaiBackground = "#2d2a2e"
	monokaiText       = "#fcfcfa"

	monokaiRed    = "#ff6188"
	monokaiOrange = "#fc9867"
	monokaiYellow = "#ffd866"
	monokaiGreen  = "#a9dc76"
	monokaiBlue   = "#78dce8"
	monokaiPurple = "#ab9df2"

	monokaiDimmed1 = "#c1c0c0"
	monokaiDimmed2 = "#939293"
	monokaiDimmed3 = "#727072"
	monokaiDimmed4 = "#5b595c"
	monokaiDimmed5 = "#403e41"
)

// monokaiChromaStyle names the chroma style whose syntax colors match this
// palette, so fenced code blocks stay inside the scheme.
const monokaiChromaStyle = "monokai"
