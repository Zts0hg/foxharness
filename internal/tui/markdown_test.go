package tui

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexMarkdownRendersHeadingsListsAndInlineStyles(t *testing.T) {
	plain := markdownPlain("# Heading\n\n1. Ordered item\n- [x] Done item\n- [ ] Todo item\n- Item with `code`\n  - Nested **bold**\n\n> Quote with *emphasis*\n\n---\n\n~~removed~~", 80)

	for _, want := range []string{
		"# Heading",
		"1. Ordered item",
		"- [x] Done item",
		"- [ ] Todo item",
		"- Item with code",
		"    - Nested bold",
		"> Quote with emphasis",
		"--------",
		"removed",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, plain)
		}
	}
	for _, forbidden := range []string{"• Item", "`code`", "**bold**", "*emphasis*", "~~removed~~"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("rendered markdown contains non-Codex marker %q:\n%s", forbidden, plain)
		}
	}
}

func TestCodexMarkdownRendersLinksUsingDestinations(t *testing.T) {
	cwd := currentTestWorkDir(t)
	target := filepath.Join(cwd, "internal", "tui", "markdown.go") + ":74"

	plain := markdownPlain("See [docs](https://example.com/docs), [markdown]("+target+"), and [relative](./internal/tui/markdown.go:75).", 100)

	for _, want := range []string{
		"docs (https://example.com/docs)",
		"internal/tui/markdown.go:74",
		"internal/tui/markdown.go:75",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered markdown missing %q:\n%s", want, plain)
		}
	}
	for _, forbidden := range []string{"[docs]", target, "./internal/tui/markdown.go"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("rendered markdown contains raw link text %q:\n%s", forbidden, plain)
		}
	}
}

func TestCodexMarkdownKeepsCodeBlocksUnwrapped(t *testing.T) {
	plain := markdownPlain("```go\nfmt.Println(\"this is a deliberately long code line that should not wrap like prose\")\n```\n", 32)

	want := `fmt.Println("this is a deliberately long code line that should not wrap like prose")`
	if !strings.Contains(plain, want) {
		t.Fatalf("rendered code block missing unwrapped line %q:\n%s", want, plain)
	}
	if strings.Contains(plain, "deliberately long code\n") {
		t.Fatalf("rendered code block appears wrapped:\n%s", plain)
	}
}

func TestCodexMarkdownUnwrapsMarkdownFenceTables(t *testing.T) {
	plain := markdownPlain("```markdown\n| Name | Value |\n| --- | ---: |\n| files | 242 |\n```\n", 80)

	for _, want := range []string{"Name", "Value", "files", "242"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered table missing %q:\n%s", want, plain)
		}
	}
	for _, forbidden := range []string{"```markdown", "| Name | Value |", "| --- | ---: |"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("rendered table contains raw fenced markdown %q:\n%s", forbidden, plain)
		}
	}
}

func TestCodexMarkdownUsesKeyValueTableFallbackWhenGridIsUnreadable(t *testing.T) {
	plain := markdownPlain("| Session | Why useful | Count |\n| --- | --- | ---: |\n| /Users/example/.codex/sessions/2026/05/25/rollout-abcdef.jsonl | The large gallery from this thread with links and emphasis | 7 |\n", 42)

	for _, want := range []string{
		"Session",
		"  /Users/example/.codex/",
		"Why useful",
		"  The large gallery from this thread",
		"Count",
		"  7",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered key/value table missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "| Session |") {
		t.Fatalf("rendered narrow table should not use raw pipe grid:\n%s", plain)
	}
}

func TestCodexMarkdownWrapsStyledTextWithoutLeakingANSI(t *testing.T) {
	rendered := renderMarkdown("**alpha beta gamma delta epsilon zeta**", 18)
	lines := strings.Split(rendered, "\n")
	if len(lines) < 2 {
		t.Fatalf("styled markdown did not wrap:\n%s", rendered)
	}
	for _, line := range lines {
		if strings.Contains(line, "\x1b[") && !strings.Contains(line, "\x1b[0m") {
			t.Fatalf("wrapped styled line is missing ANSI reset:\n%q\nfull render:\n%s", line, rendered)
		}
	}
}

func markdownPlain(markdown string, width int) string {
	return stripANSI(renderMarkdown(markdown, width))
}

func currentTestWorkDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve cwd: %v", err)
	}
	return dir
}
