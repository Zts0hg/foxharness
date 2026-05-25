package skilltool

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/slash"
)

func mkCmd(name, desc string) *slash.Command {
	return &slash.Command{
		Type:        slash.CommandPrompt,
		Name:        name,
		Description: desc,
		Frontmatter: slash.Frontmatter{UserInvocable: true},
	}
}

func mkBuiltin(name, desc string) *slash.Command {
	return &slash.Command{
		Type:        slash.CommandBuiltin,
		Name:        name,
		Description: desc,
		Frontmatter: slash.Frontmatter{UserInvocable: true},
	}
}

func TestFormatSkillsWithinBudget_EmptyList(t *testing.T) {
	got := FormatSkillsWithinBudget(nil, 100000)
	if got != "" {
		t.Errorf("expected empty for empty list, got %q", got)
	}
}

func TestFormatSkillsWithinBudget_NoTruncation(t *testing.T) {
	cmds := []*slash.Command{
		mkCmd("review", "Review code for quality issues"),
		mkCmd("deploy", "Deploy the application"),
	}
	out := FormatSkillsWithinBudget(cmds, 200000)
	if !strings.Contains(out, "review") || !strings.Contains(out, "Review code for quality issues") {
		t.Errorf("missing full review entry: %q", out)
	}
	if !strings.Contains(out, "deploy") || !strings.Contains(out, "Deploy the application") {
		t.Errorf("missing full deploy entry: %q", out)
	}
}

func TestFormatSkillsWithinBudget_NormalTruncation(t *testing.T) {
	longDesc := strings.Repeat("longdesc ", 30)
	cmds := []*slash.Command{
		mkCmd("a", longDesc),
		mkCmd("b", longDesc),
		mkCmd("c", longDesc),
		mkCmd("d", longDesc),
		mkCmd("e", longDesc),
	}
	// Pick a budget small enough to force shrinking but big enough for names.
	out := FormatSkillsWithinBudget(cmds, 500)
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		if !strings.Contains(out, "- "+name) {
			t.Errorf("name %s missing", name)
		}
	}
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 250 {
			t.Errorf("line exceeded normal cap: %d chars", len(line))
		}
	}
}

func TestFormatSkillsWithinBudget_ExtremeTruncation(t *testing.T) {
	cmds := []*slash.Command{
		mkCmd("aa", "long descriptions can't fit"),
		mkCmd("bb", "long descriptions can't fit"),
	}
	out := FormatSkillsWithinBudget(cmds, 1) // ~40 char budget
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "long descriptions") {
			t.Errorf("descriptions should be stripped under extreme truncation: %q", line)
		}
	}
	if !strings.Contains(out, "- aa") || !strings.Contains(out, "- bb") {
		t.Errorf("expected name-only entries, got %q", out)
	}
}

func TestFormatSkillsWithinBudget_BuiltinPreserved(t *testing.T) {
	cmds := []*slash.Command{
		mkBuiltin("builtin", "Built-in preserved verbose description"),
		mkCmd("user", strings.Repeat("xx ", 60)),
		mkCmd("user2", strings.Repeat("xx ", 60)),
	}
	out := FormatSkillsWithinBudget(cmds, 10)
	if !strings.Contains(out, "Built-in preserved verbose description") {
		t.Errorf("builtin description must survive truncation: %q", out)
	}
}

func TestFormatSkillsWithinBudget_DescriptionCap(t *testing.T) {
	huge := strings.Repeat("x", 1000)
	cmds := []*slash.Command{mkCmd("a", huge)}
	out := FormatSkillsWithinBudget(cmds, 200000)
	if strings.Count(out, "x") > 250 {
		t.Errorf("per-skill description was not capped at 250 chars")
	}
}

func TestFormatSkillsWithinBudget_UnknownContextWindow(t *testing.T) {
	cmds := []*slash.Command{mkCmd("a", "abc")}
	out := FormatSkillsWithinBudget(cmds, 0)
	if !strings.Contains(out, "- a") {
		t.Errorf("fallback budget should still include the skill: %q", out)
	}
}

func TestFormatSkillsWithinBudget_IncludesWhenToUseAndHint(t *testing.T) {
	c := mkCmd("review", "review")
	c.Frontmatter.WhenToUse = "Use when reviewing code"
	c.Frontmatter.ArgumentHint = "[file]"
	out := FormatSkillsWithinBudget([]*slash.Command{c}, 200000)
	if !strings.Contains(out, "Use when reviewing code") {
		t.Errorf("when_to_use missing: %q", out)
	}
	if !strings.Contains(out, "[file]") {
		t.Errorf("argument-hint missing: %q", out)
	}
}
