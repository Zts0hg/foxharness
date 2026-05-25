package skilltool

import (
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/slash"
)

// Budgeting constants modeled after Claude Code's skill list formatter.
const (
	bytesPerToken       = 4
	budgetRatio         = 0.01
	fallbackBudgetChars = 8_000
	maxDescriptionChars = 250
	minDescriptionChars = 20
)

// FormatSkillsWithinBudget returns the formatted skill list to embed in
// the system prompt. The output respects a character budget derived from
// the model's context window (1% of the window, expressed in characters).
//
// Three truncation levels are applied:
//
//  1. No truncation — when the total length of all entries fits inside
//     the budget, every skill is shown with its full description.
//  2. Normal truncation — when names alone fit but full descriptions do
//     not, the remaining budget is distributed evenly across non-builtin
//     skill descriptions (min 20 chars each).
//  3. Extreme truncation — when even minimal descriptions exceed the
//     budget, non-builtin skills are shown as name-only entries.
//
// Built-in skills always retain their full descriptions; they are not
// subject to truncation.
func FormatSkillsWithinBudget(commands []*slash.Command, contextWindowTokens int) string {
	if len(commands) == 0 {
		return ""
	}
	budget := charBudget(contextWindowTokens)

	full := renderFull(commands)
	if totalLen(full) <= budget {
		return strings.Join(full, "\n")
	}

	// Compute name-only baseline length for non-builtins; builtins keep
	// their full description.
	var (
		builtinLen   int
		nonBuiltins  []*slash.Command
		nonBuiltinNm []string
	)
	for _, cmd := range commands {
		if cmd.Type == slash.CommandBuiltin {
			builtinLen += len(renderEntry(cmd, cmd.Description))
		} else {
			nonBuiltins = append(nonBuiltins, cmd)
			nonBuiltinNm = append(nonBuiltinNm, fmt.Sprintf("- %s", cmd.Name))
		}
	}

	remaining := budget - builtinLen
	for _, name := range nonBuiltinNm {
		remaining -= len(name) + 1 // +1 for newline
	}

	if remaining < minDescriptionChars*len(nonBuiltins) {
		// Extreme: name-only for non-builtins
		var lines []string
		for _, cmd := range commands {
			if cmd.Type == slash.CommandBuiltin {
				lines = append(lines, renderEntry(cmd, cmd.Description))
			} else {
				lines = append(lines, fmt.Sprintf("- %s", cmd.Name))
			}
		}
		return strings.Join(lines, "\n")
	}

	// Normal truncation — evenly distribute remaining chars across non-builtins.
	perSkill := remaining / max1(len(nonBuiltins))
	if perSkill > maxDescriptionChars {
		perSkill = maxDescriptionChars
	}
	if perSkill < minDescriptionChars {
		perSkill = minDescriptionChars
	}
	var lines []string
	for _, cmd := range commands {
		if cmd.Type == slash.CommandBuiltin {
			lines = append(lines, renderEntry(cmd, cmd.Description))
			continue
		}
		desc := truncate(cmd.Description, perSkill)
		lines = append(lines, renderEntry(cmd, desc))
	}
	return strings.Join(lines, "\n")
}

func charBudget(contextWindowTokens int) int {
	if contextWindowTokens <= 0 {
		return fallbackBudgetChars
	}
	return int(float64(contextWindowTokens*bytesPerToken) * budgetRatio)
}

func renderFull(cmds []*slash.Command) []string {
	out := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		desc := cmd.Description
		if len(desc) > maxDescriptionChars {
			desc = desc[:maxDescriptionChars]
		}
		out = append(out, renderEntry(cmd, desc))
	}
	return out
}

func renderEntry(cmd *slash.Command, desc string) string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "- %s", cmd.Name)
	if desc != "" {
		fmt.Fprintf(b, ": %s", desc)
	}
	if cmd.Frontmatter.WhenToUse != "" {
		fmt.Fprintf(b, " (%s)", cmd.Frontmatter.WhenToUse)
	}
	if cmd.Frontmatter.ArgumentHint != "" {
		fmt.Fprintf(b, " %s", cmd.Frontmatter.ArgumentHint)
	}
	return b.String()
}

func totalLen(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

func max1(n int) int {
	if n < 1 {
		return 1
	}
	return n
}
