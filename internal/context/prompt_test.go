package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSkillMarkdownFrontmatter(t *testing.T) {
	content := `---
name: go-refactor
description: Use this skill for Go refactoring.
---

# Guide

Read tests before editing production code.
`

	skill := parseSkillMarkdown("refactor", content)
	if skill.RequestedName != "refactor" {
		t.Fatalf("RequestedName = %q, want refactor", skill.RequestedName)
	}
	if skill.Name != "go-refactor" {
		t.Fatalf("Name = %q, want go-refactor", skill.Name)
	}
	if skill.Description != "Use this skill for Go refactoring." {
		t.Fatalf("Description = %q", skill.Description)
	}
	if !strings.Contains(skill.Content, "Read tests before editing production code.") {
		t.Fatalf("Content did not include body: %q", skill.Content)
	}
	if strings.Contains(skill.Content, "description:") {
		t.Fatalf("Content still contains frontmatter: %q", skill.Content)
	}
}

func TestComposeLoadsMentionedSkillWithParsedFrontmatter(t *testing.T) {
	workDir := t.TempDir()
	skillDir := filepath.Join(workDir, ".foxharness", "skills", "go-refactor")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillContent := `---
name: Go Refactor
description: Use for focused Go refactoring tasks.
---

Rules:
- Read related tests first.
- Run the smallest relevant go test command.
`
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).Compose("请使用 $go-refactor 改一下代码")
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"## Loaded Skill: Go Refactor",
		"Requested as: $go-refactor",
		"Description:",
		"Use for focused Go refactoring tasks.",
		"Read related tests first.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "name: Go Refactor") {
		t.Fatalf("prompt still contains raw frontmatter:\n%s", prompt)
	}
}

func TestComposeDoesNotLoadUnmentionedSkill(t *testing.T) {
	workDir := t.TempDir()
	skillDir := filepath.Join(workDir, ".foxharness", "skills", "go-refactor")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Should not load"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).Compose("普通任务，不点名技能")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "Should not load") {
		t.Fatalf("unmentioned skill was loaded:\n%s", prompt)
	}
}
