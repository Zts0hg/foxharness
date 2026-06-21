package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/automemory"
)

func TestComposeInteractiveAskGuidance(t *testing.T) {
	workDir := t.TempDir()

	enabled, err := NewComposer(workDir).WithInteractiveAsk(true).Compose("普通任务")
	if err != nil {
		t.Fatalf("Compose(enabled) error = %v", err)
	}
	if !strings.Contains(enabled, "ask_user_question") {
		t.Fatalf("interactive guidance missing when enabled:\n%s", enabled)
	}

	disabled, err := NewComposer(workDir).WithInteractiveAsk(false).Compose("普通任务")
	if err != nil {
		t.Fatalf("Compose(disabled) error = %v", err)
	}
	if strings.Contains(disabled, "ask_user_question") {
		t.Fatalf("interactive guidance must be omitted when disabled:\n%s", disabled)
	}

	// Default (no WithInteractiveAsk) must also omit it.
	def, err := NewComposer(workDir).Compose("普通任务")
	if err != nil {
		t.Fatalf("Compose(default) error = %v", err)
	}
	if strings.Contains(def, "ask_user_question") {
		t.Fatalf("interactive guidance must be off by default:\n%s", def)
	}
}

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

func TestComposeNoLongerInjectsLegacyProjectMemory(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	// A legacy flat MEMORY.md must be ignored now (REQ-017 / CON-002).
	if err := os.WriteFile(filepath.Join(workDir, "MEMORY.md"), []byte("legacy project convention"), 0o644); err != nil {
		t.Fatal(err)
	}
	workingMemoryPath := filepath.Join(sessionDir, "working_memory.md")
	if err := os.WriteFile(workingMemoryPath, []byte("session note"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).WithMemory(workingMemoryPath).Compose("普通任务")
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{
		"## Project Memory from MEMORY.md",
		"legacy project convention",
	} {
		if strings.Contains(prompt, absent) {
			t.Fatalf("prompt must no longer inject legacy MEMORY.md, found %q:\n%s", absent, prompt)
		}
	}
	// Working memory is still injected.
	for _, want := range []string{"## Session Working Memory", "session note"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestComposeInjectsPersistentMemoryIndexAndGuardrails(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	store := automemory.NewStore(home, workDir)
	if err := store.Save(automemory.Memory{
		Name:        "user-role",
		Description: "Staff engineer, terse answers.",
		Type:        automemory.TypeUser,
		Body:        "The user is a staff engineer.",
	}); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).WithAutoMemory(store).Compose("普通任务")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Persistent Memory",
		"user-role.md",  // merged index entry (REQ-006)
		"Do NOT save",   // guardrail (REQ-014)
		"ignore memory", // ignore directive (REQ-014)
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Project Memory from MEMORY.md") {
		t.Fatalf("legacy MEMORY.md section must be gone:\n%s", prompt)
	}
}

// TestSC001MemoryPersistsAcrossSessionsSameProject covers SC-001: a memory saved
// in one session is observable in the injected index of a later session in the
// same project.
func TestSC001MemoryPersistsAcrossSessionsSameProject(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()

	// Session 1 saves a project memory.
	session1 := automemory.NewStore(home, workDir)
	if err := session1.Save(automemory.Memory{
		Name:        "proj-build",
		Description: "Build with make build.",
		Type:        automemory.TypeProject,
		Body:        "Use make build.\n\n**Why:** wraps codegen.\n**How to apply:** run make build before go build.",
	}); err != nil {
		t.Fatal(err)
	}

	// A fresh session in the same project (new Store, same home+workDir) sees it.
	session2 := automemory.NewStore(home, workDir)
	prompt, err := NewComposer(workDir).WithAutoMemory(session2).Compose("task")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "proj-build.md") {
		t.Fatalf("SC-001: memory not visible in new session's injected index:\n%s", prompt)
	}
}

// TestSC002UserMemoryVisibleAcrossProjects covers SC-002: a user memory written
// while working in project A is observable in project B's injected index, while a
// project-scoped memory from A does not leak to B.
func TestSC002UserMemoryVisibleAcrossProjects(t *testing.T) {
	home := t.TempDir()
	projectA := t.TempDir()
	projectB := t.TempDir()

	storeA := automemory.NewStore(home, projectA)
	if err := storeA.Save(automemory.Memory{
		Name:        "user-role",
		Description: "Staff engineer, terse answers.",
		Type:        automemory.TypeUser,
		Body:        "The user is a staff engineer.",
	}); err != nil {
		t.Fatal(err)
	}
	if err := storeA.Save(automemory.Memory{
		Name:        "proj-a-only",
		Description: "Project A specific.",
		Type:        automemory.TypeProject,
		Body:        "x\n\n**Why:** w\n**How to apply:** h",
	}); err != nil {
		t.Fatal(err)
	}

	storeB := automemory.NewStore(home, projectB)
	prompt, err := NewComposer(projectB).WithAutoMemory(storeB).Compose("task")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "user-role.md") {
		t.Fatalf("SC-002: user memory not visible in project B:\n%s", prompt)
	}
	if strings.Contains(prompt, "proj-a-only.md") {
		t.Fatalf("SC-002: project A memory leaked into project B:\n%s", prompt)
	}
}

func TestComposeOmitsPersistentMemoryWhenStoreUnset(t *testing.T) {
	prompt, err := NewComposer(t.TempDir()).Compose("普通任务")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "## Persistent Memory") {
		t.Fatalf("persistent memory section must be omitted when no store is set:\n%s", prompt)
	}
}

func TestComposeIncludesWorkingMemoryMaintenanceGuidance(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	workingMemoryPath := filepath.Join(sessionDir, "working_memory.md")
	if err := os.WriteFile(workingMemoryPath, []byte("# Working Memory\n\n## Goal\n\nNot recorded.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).WithMemory(workingMemoryPath).Compose("普通任务")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Session Working Memory",
		"session-scoped",
		"Goal",
		"Known Facts",
		"Current Plan",
		"Next Step",
		"write_file",
		"edit_file",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("working memory guidance missing %q:\n%s", want, prompt)
		}
	}
	// The guidance must name the workDir-relative path to the session file so the
	// agent's write_file/edit_file land in the injected scratchpad, not in
	// <workDir>/working_memory.md (P2).
	rel, err := filepath.Rel(workDir, workingMemoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, rel) {
		t.Fatalf("working memory guidance must include the workDir-relative path %q:\n%s", rel, prompt)
	}
}

func TestComposeWorkingMemoryPathResolvesBackToSessionFile(t *testing.T) {
	workDir := t.TempDir()
	// A session-style layout: workDir's sibling tree under the same home root.
	sessionDir := filepath.Join(filepath.Dir(workDir), ".foxharness", "projects", "key", "sessions", "1")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	workingMemoryPath := filepath.Join(sessionDir, "working_memory.md")
	if err := os.WriteFile(workingMemoryPath, []byte("# Working Memory\n\n## Goal\n\ngoal\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).WithMemory(workingMemoryPath).Compose("task")
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(workDir, workingMemoryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, rel) {
		t.Fatalf("guidance missing relative path %q", rel)
	}
	// Joined back with workDir, the relative path must resolve to the session file.
	resolved := filepath.Join(workDir, rel)
	if resolved != workingMemoryPath {
		t.Fatalf("rel path resolves to %q, want %q", resolved, workingMemoryPath)
	}
}

func TestComposeIncludesTodoToolInstructions(t *testing.T) {
	prompt, err := NewComposer(t.TempDir()).Compose("普通任务")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Use read_todo and update_todo to inspect and maintain Session TODO.md.",
		"Do not use bash, write_file, or edit_file to modify Session TODO.md.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
