package keeprun

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	maxSlugLength = 60
	fallbackSlug  = "task"
)

var typePrefixPattern = regexp.MustCompile(`^\s*\[[^\]]*\]\s*`)

// typePrefixMap maps task type labels from backlog headings to short branch
// name prefixes. Unknown or empty types map to "misc".
var typePrefixMap = map[string]string{
	"feature":  "feat",
	"fix":      "fix",
	"refactor": "refactor",
	"docs":     "docs",
	"chore":    "chore",
	"test":     "test",
}

// mapTypePrefix returns the short prefix for a task type label. Unknown or
// empty types map to "misc".
func mapTypePrefix(taskType string) string {
	if taskType == "" {
		return "misc"
	}
	prefix, ok := typePrefixMap[strings.ToLower(strings.TrimSpace(taskType))]
	if !ok {
		return "misc"
	}
	return prefix
}

// GenerateSlug converts a task heading like "[feature] Add dark mode" into a
// filesystem-safe slug such as "add-dark-mode".
//
// It implements steps 1-7 of the slug algorithm defined in spec FR-005:
//
//  1. Take the task title text.
//  2. Strip a leading "[type]" prefix if present.
//  3. Convert to lowercase.
//  4. Replace any character outside [a-z0-9] with a hyphen.
//  5. Collapse consecutive hyphens into a single hyphen.
//  6. Strip leading and trailing hyphens.
//  7. Truncate to at most maxSlugLength characters, breaking at the last
//     hyphen boundary when one exists within the limit.
//
// Step 8 (collision deduplication) is implemented separately by DeduplicateSlug.
//
// When the title reduces to an empty string — for example a title made up
// entirely of punctuation or a bare "[type]" prefix — GenerateSlug returns the
// fallback slug "task" so that callers always receive a valid branch name.
func GenerateSlug(title string) string {
	stripped := typePrefixPattern.ReplaceAllString(title, "")
	lowered := strings.ToLower(stripped)

	var b strings.Builder
	b.Grow(len(lowered))
	for _, r := range lowered {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}

	slug := collapseHyphens(b.String())
	slug = strings.Trim(slug, "-")
	slug = truncateSlug(slug, maxSlugLength)
	slug = strings.Trim(slug, "-")

	if slug == "" {
		return fallbackSlug
	}
	return slug
}

// GenerateSlugWithTypePrefix converts a task heading like "[feature] Add dark mode"
// into a filesystem-safe slug with a type prefix such as "feat-add-dark-mode".
//
// It extends the slug algorithm to prepend a short type prefix (e.g., "feat" for
// "feature") to avoid collisions between tasks with the same title but different
// types. The mapping is:
//
//	feature   → feat
//	fix       → fix
//	refactor  → refactor
//	docs      → docs
//	chore     → chore
//	test      → test
//	(unknown) → misc
//
// The prefix is prepended to the kebab-case title, and the total result is
// truncated to maxSlugLength characters. Truncation preserves the prefix and
// breaks at hyphen boundaries when possible.
func GenerateSlugWithTypePrefix(title string, taskType string) string {
	// Extract the title content without [type] bracket prefix.
	contentTitle := typePrefixPattern.ReplaceAllString(title, "")
	prefix := mapTypePrefix(taskType)

	// Convert content to kebab-case.
	var b strings.Builder
	b.Grow(len(prefix) + 1 + len(contentTitle))
	for _, r := range strings.ToLower(contentTitle) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}

	slug := prefix + "-" + collapseHyphens(b.String())
	slug = strings.Trim(slug, "-")
	slug = truncateSlug(slug, maxSlugLength)
	slug = strings.Trim(slug, "-")

	if slug == "" {
		return fallbackSlug
	}
	return slug
}

// DeduplicateSlug implements step 8 of the slug algorithm from spec FR-005: on
// collision with an existing branch name, it appends a numeric suffix (-2, -3,
// ...) until the result is unique. If slug does not appear in existing it is
// returned unchanged. The suffix is appended rather than replacing any trailing
// number already present in slug, so a meaningful slug like "fix-bug-2" keeps
// its number and becomes "fix-bug-2-2" on collision.
func DeduplicateSlug(slug string, existing []string) string {
	taken := make(map[string]bool, len(existing))
	for _, name := range existing {
		taken[name] = true
	}
	if !taken[slug] {
		return slug
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", slug, i)
		if !taken[candidate] {
			return candidate
		}
	}
}

// collapseHyphens replaces every run of consecutive hyphens with a single one.
func collapseHyphens(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevHyphen := false
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			if prevHyphen {
				continue
			}
			prevHyphen = true
		} else {
			prevHyphen = false
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// truncateSlug shortens s to at most max bytes. When the truncated prefix still
// contains a hyphen, it cuts back to that last hyphen so the slug ends on a word
// boundary; otherwise it hard-truncates a single oversized word. The slug only
// contains ASCII [a-z0-9-] at this point, so byte indexing is safe.
func truncateSlug(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	if idx := strings.LastIndex(cut, "-"); idx > 0 {
		return cut[:idx]
	}
	return cut
}
