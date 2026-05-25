package slash

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParseArguments splits a raw argument string into individual arguments,
// honoring double-quoted groupings (e.g. "hello world" stays one arg). An
// unterminated quote captures the rest of the input as a single argument.
//
// This is intentionally simpler than POSIX shell parsing: no single quotes,
// no escapes. The slash command surface does not need full shell semantics
// — only space-splitting with quoted groups.
func ParseArguments(input string) []string {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var (
		out     []string
		current strings.Builder
		inQuote bool
	)
	flush := func() {
		if current.Len() > 0 {
			out = append(out, current.String())
			current.Reset()
		}
	}
	for _, r := range input {
		switch {
		case r == '"':
			inQuote = !inQuote
		case !inQuote && (r == ' ' || r == '\t'):
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return out
}

// SplitArgumentNames parses the frontmatter `arguments` field into an
// ordered list of named parameters. Names are space-separated.
func SplitArgumentNames(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	return parts
}

var (
	argsBracketRe    = regexp.MustCompile(`\$ARGUMENTS\[(\d+)\]`)
	argsIndexedRe    = regexp.MustCompile(`\$(\d+)`)
	argsNamedRe      = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)
	argsPlaceholders = regexp.MustCompile(`\$ARGUMENTS\[\d+\]|\$ARGUMENTS|\$\d+|\$[A-Za-z_][A-Za-z0-9_]*`)
)

// SubstituteArguments walks content and replaces argument placeholders with
// the values from args (or empty strings when an index is out of range).
// argNames is the ordered list of named parameters declared in the
// frontmatter `arguments` field; named placeholders that do not appear in
// argNames are left untouched.
//
// If the content does not contain any placeholder but the user provided
// arguments, the raw arguments are appended to the end of the content as a
// fallback so that the command body still has access to the user's input.
func SubstituteArguments(content string, args []string, argNames []string) string {
	hasPlaceholder := argsPlaceholders.MatchString(content)

	out := content
	out = argsBracketRe.ReplaceAllStringFunc(out, func(match string) string {
		sub := argsBracketRe.FindStringSubmatch(match)
		idx, _ := strconv.Atoi(sub[1])
		return argAt(args, idx)
	})
	out = strings.ReplaceAll(out, "$ARGUMENTS", strings.Join(args, " "))
	out = argsIndexedRe.ReplaceAllStringFunc(out, func(match string) string {
		sub := argsIndexedRe.FindStringSubmatch(match)
		idx, _ := strconv.Atoi(sub[1])
		return argAt(args, idx)
	})
	if len(argNames) > 0 {
		out = argsNamedRe.ReplaceAllStringFunc(out, func(match string) string {
			name := match[1:]
			for i, n := range argNames {
				if n == name {
					return argAt(args, i)
				}
			}
			return match
		})
	}

	if !hasPlaceholder && len(args) > 0 {
		out = fmt.Sprintf("%s\n\nARGUMENTS: %s", out, strings.Join(args, " "))
	}
	return out
}

func argAt(args []string, idx int) string {
	if idx < 0 || idx >= len(args) {
		return ""
	}
	return args[idx]
}

// ProgressiveHint returns the autocomplete hint shown to the user as they
// type a command's arguments. customHint, when non-empty, takes precedence
// over the auto-generated `[name]` list. Once filledCount has consumed all
// declared argNames, the hint becomes empty.
func ProgressiveHint(argNames []string, filledCount int, customHint string) string {
	if customHint != "" {
		return customHint
	}
	if filledCount >= len(argNames) {
		return ""
	}
	remaining := argNames[filledCount:]
	pieces := make([]string, len(remaining))
	for i, name := range remaining {
		pieces[i] = "[" + name + "]"
	}
	return strings.Join(pieces, " ")
}
