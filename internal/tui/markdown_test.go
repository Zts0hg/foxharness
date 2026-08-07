package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestCodexMarkdownRendersHeadingsListsAndInlineStyles(t *testing.T) {
	rendered := renderMarkdown("# Heading\n\n1. Ordered item\n- [x] Done item\n- [ ] Todo item\n- Item with `code`\n  - Nested **bold**\n\n> Quote with *emphasis*\n\n---\n\n~~removed~~", 80)
	plain := stripANSI(rendered)

	for _, want := range []string{
		"# Heading",
		"1. Ordered item",
		"- [x] Done item",
		"- [ ] Todo item",
		"- Item with code",
		"    - Nested bold",
		"> Quote with emphasis",
		"────────",
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
	if !strings.Contains(rendered, "\x1b[3m") {
		t.Fatalf("rendered markdown missing italic ANSI styling:\n%s", rendered)
	}
	if !strings.Contains(rendered, "\x1b[9m") {
		t.Fatalf("rendered markdown missing strikethrough ANSI styling:\n%s", rendered)
	}
}

func TestCodexMarkdownUsesCodexAnsiStyles(t *testing.T) {
	rendered := renderMarkdown("> [link](https://example.com) and `code`", 80)

	for _, want := range []string{
		"\x1b[32m",
		"\x1b[36m",
		"\x1b[4;",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered markdown missing ANSI style %q:\n%s", want, rendered)
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
	rendered := renderMarkdown("```go\nfmt.Println(\"this is a deliberately long code line that should not wrap like prose\")\n```\n", 32)
	plain := stripANSI(rendered)

	want := `fmt.Println("this is a deliberately long code line that should not wrap like prose")`
	if !strings.Contains(plain, want) {
		t.Fatalf("rendered code block missing unwrapped line %q:\n%s", want, plain)
	}
	if strings.Contains(plain, "deliberately long code\n") {
		t.Fatalf("rendered code block appears wrapped:\n%s", plain)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("rendered code block missing syntax or code ANSI styling:\n%s", rendered)
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

func TestCodexMarkdownPreservesNonTableMarkdownFences(t *testing.T) {
	plain := markdownPlain("```markdown\n# Example\nnot a table\n```\n", 80)

	for _, want := range []string{"# Example", "not a table"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered code block missing %q:\n%s", want, plain)
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

func TestCodexMarkdownTableUsesAvailableWidth(t *testing.T) {
	plain := markdownPlain("| 项目 | 状态 |\n| --- | --- |\n| Markdown 转换 | ✅ 16 篇全部重新生成到 reference/geektime/996695/markdown/ |\n", 96)

	if !strings.Contains(plain, "reference/geektime/996695/markdown/") {
		t.Fatalf("wide table wrapped a path despite available width:\n%s", plain)
	}
	for _, line := range strings.Split(plain, "\n") {
		if got := lipgloss.Width(line); got > 96 {
			t.Fatalf("rendered table line width = %d, want <= 96: %q\n%s", got, line, plain)
		}
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

func TestMarkdownStreamCommitsCompleteLinesAndFinalizesTail(t *testing.T) {
	stream := newMarkdownStreamController(80)

	if got := stream.Push("alpha"); got != "" {
		t.Fatalf("partial push rendered %q, want empty stable output", got)
	}
	if got := stripANSI(stream.Push(" beta\nnext")); !strings.Contains(got, "• alpha beta") {
		t.Fatalf("stream did not commit complete line:\n%s", got)
	}
	if got := stripANSI(stream.Finish()); !strings.Contains(got, "next") {
		t.Fatalf("stream finish did not render tail:\n%s", got)
	}
}

func TestTerminalHyperlinksDoNotAffectVisibleWidth(t *testing.T) {
	line := lineWithHyperlinks{
		text: "docs",
		links: []terminalHyperlink{{
			start: 0,
			end:   4,
			url:   "https://example.com/docs",
		}},
	}

	rendered := line.render()
	if lipgloss.Width(rendered) != 4 {
		t.Fatalf("hyperlink width = %d, want 4: %q", lipgloss.Width(rendered), rendered)
	}
	if !strings.Contains(rendered, "\x1b]8;;https://example.com/docs\x1b\\") {
		t.Fatalf("rendered hyperlink missing OSC 8 opener: %q", rendered)
	}
	if !strings.HasSuffix(rendered, "\x1b]8;;\x1b\\") {
		t.Fatalf("rendered hyperlink missing OSC 8 closer: %q", rendered)
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
