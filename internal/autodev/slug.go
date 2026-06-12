package autodev

import (
	"fmt"
	"strings"
)

// maxSlugLength bounds slugs so branch names and worktree paths stay short.
const maxSlugLength = 64

// Slug derives a kebab-case identifier from an item title. ASCII letters and
// digits are lowercased, every other run of characters collapses to a single
// hyphen, and the result is truncated to maxSlugLength. When the derived
// slug is already present in taken, a numeric suffix (-2, -3, ...)
// disambiguates it so branch and worktree paths stay unique.
func Slug(title string, taken map[string]bool) string {
	base := kebab(title)
	if base == "" {
		base = "item"
	}
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !taken[candidate] {
			return candidate
		}
	}
}

func kebab(title string) string {
	var b strings.Builder
	lastHyphen := true
	for _, r := range strings.ToLower(title) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > maxSlugLength {
		out = strings.Trim(out[:maxSlugLength], "-")
	}
	return out
}
