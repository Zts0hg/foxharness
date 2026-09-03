package tui

import (
	"math"
	"strconv"
	"strings"
)

/*
Package-local conversion from a truecolor value to an xterm-256 palette index.

termenv v0.16.0 does this conversion itself, but derives its grayscale
candidate from the 6x6x6 cube indices instead of the channel values
(hexToANSI256Color computes `average := (r + g + b) / 3` where r, g and b are
already 0..5), so the candidate is always index 232 and the 232-255 gray ramp
is unreachable. Every near-neutral color is therefore forced into the cube,
whose dark band is one bucket wide from 48 to 114 — which collapses the dimmed
text, divider, and progress-track colors of a fixed dark theme onto a single
index and erases the hierarchy between them.

Resolving the index here keeps those steps distinct and lands every color
closer to its published value, without changing the palette a truecolor
terminal receives.
*/

// ansi256CubeLevels are the six channel values of the xterm 6x6x6 color cube.
var ansi256CubeLevels = [6]int{0, 0x5f, 0x87, 0xaf, 0xd7, 0xff}

// ansi256FirstFixedIndex is where the terminal-independent part of the palette
// begins. Indices 0-15 render with whatever the user configured in their
// terminal, so a fixed theme must never resolve to one.
const ansi256FirstFixedIndex = 16

// nearestANSI256 returns the palette index whose color is perceptually closest
// to a "#rrggbb" value, and whether the value was a truecolor one at all.
// Empty strings and ANSI indices — what a terminal-following theme carries —
// report false and must be left as they are.
func nearestANSI256(value string) (string, bool) {
	r, g, b, ok := parseHexColor(value)
	if !ok {
		return "", false
	}
	target := srgbToLab(r, g, b)
	bestIndex := ansi256FirstFixedIndex
	bestDistance := math.MaxFloat64
	for index := ansi256FirstFixedIndex; index < 256; index++ {
		er, eg, eb := ansi256Entry(index)
		if distance := labDistance(target, srgbToLab(er, eg, eb)); distance < bestDistance {
			bestDistance = distance
			bestIndex = index
		}
	}
	return strconv.Itoa(bestIndex), true
}

// ansi256Entry returns the RGB channels of a palette index at or above 16: the
// 6x6x6 color cube for 16-231, then the 24-step gray ramp for 232-255.
func ansi256Entry(index int) (int, int, int) {
	if index >= 232 {
		value := 8 + 10*(index-232)
		return value, value, value
	}
	offset := index - 16
	return ansi256CubeLevels[offset/36], ansi256CubeLevels[(offset/6)%6], ansi256CubeLevels[offset%6]
}

func parseHexColor(value string) (int, int, int, bool) {
	value = strings.TrimSpace(value)
	if len(value) != 7 || value[0] != '#' {
		return 0, 0, 0, false
	}
	parsed, err := strconv.ParseUint(value[1:], 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(parsed >> 16 & 0xff), int(parsed >> 8 & 0xff), int(parsed & 0xff), true
}

// srgbToLab converts an 8-bit sRGB color to CIE L*a*b* under a D65 white
// point. Nearest-neighbor search needs a perceptually uniform space; picking
// by raw RGB distance would prefer a cube entry over a much closer gray.
func srgbToLab(r, g, b int) [3]float64 {
	linear := func(v int) float64 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	lr, lg, lb := linear(r), linear(g), linear(b)
	x := (lr*0.4124 + lg*0.3576 + lb*0.1805) / 0.95047
	y := lr*0.2126 + lg*0.7152 + lb*0.0722
	z := (lr*0.0193 + lg*0.1192 + lb*0.9505) / 1.08883
	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116.0
	}
	fx, fy, fz := f(x), f(y), f(z)
	return [3]float64{116*fy - 16, 500 * (fx - fy), 200 * (fy - fz)}
}

// labDistance is the squared CIE76 color difference. The square root is
// omitted because only the ordering matters.
func labDistance(a, b [3]float64) float64 {
	dl, da, db := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return dl*dl + da*da + db*db
}
