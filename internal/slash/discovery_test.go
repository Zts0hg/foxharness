package slash

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestDiscoverCommands_SingleFile(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "review.md"), "Review the code")

	user, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(user) != 0 {
		t.Errorf("expected 0 user commands, got %d", len(user))
	}
	if len(project) != 1 {
		t.Fatalf("expected 1 project command, got %d", len(project))
	}
	if project[0].Name != "review" {
		t.Errorf("name = %q", project[0].Name)
	}
	if project[0].Source != SourceProject {
		t.Errorf("source = %v", project[0].Source)
	}
}

func TestDiscoverCommands_SkillDirectory(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeFile(t, filepath.Join(workDir, ".foxharness", "skills", "go-test", "SKILL.md"), "Run go tests")

	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(project) != 1 {
		t.Fatalf("expected 1 project command, got %d", len(project))
	}
	if project[0].Name != "go-test" {
		t.Errorf("name = %q", project[0].Name)
	}
	if project[0].SkillDir == "" {
		t.Error("SkillDir should be populated for SKILL.md")
	}
}

func TestDiscoverCommands_NamespaceMapping(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "db", "migrate.md"), "Run migrations")
	writeFile(t, filepath.Join(workDir, ".foxharness", "skills", "testing", "go-test", "SKILL.md"), "Test")

	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	names := make([]string, 0)
	for _, c := range project {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	got := strings.Join(names, ",")
	if got != "db:migrate,testing:go-test" {
		t.Errorf("namespaced names = %q", got)
	}
}

func TestDiscoverCommands_UserVsProject(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeFile(t, filepath.Join(userHome, ".foxharness", "commands", "global.md"), "global")
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "local.md"), "local")

	user, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(user) != 1 || user[0].Name != "global" {
		t.Errorf("user = %+v", user)
	}
	if user[0].Source != SourceUser {
		t.Errorf("user source = %v", user[0].Source)
	}
	if len(project) != 1 || project[0].Name != "local" {
		t.Errorf("project = %+v", project)
	}
}

func TestDiscoverCommands_MissingDirectories(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()

	user, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(user) != 0 || len(project) != 0 {
		t.Errorf("expected empty results, got user=%d project=%d", len(user), len(project))
	}
}

func TestDiscoverCommands_LooseSkillFileIgnored(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	// Loose .md file directly under skills/ — should be ignored (directory format only).
	writeFile(t, filepath.Join(workDir, ".foxharness", "skills", "loose.md"), "loose skill")
	// A proper directory-format skill should still load.
	writeFile(t, filepath.Join(workDir, ".foxharness", "skills", "proper", "SKILL.md"), "proper")

	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(project) != 1 || project[0].Name != "proper" {
		t.Errorf("expected only 'proper' to load, got %+v", project)
	}
}

func TestDiscoverCommands_LargeFileSkipped(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	// Create a file just over the 1MB threshold.
	big := strings.Repeat("a", maxCommandFileSize+1)
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "big.md"), big)
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "small.md"), "small")

	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(project) != 1 || project[0].Name != "small" {
		t.Errorf("expected only 'small' to load, got %+v", project)
	}
}

func TestDiscoverCommands_FrontmatterApplied(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	body := `---
description: "review code"
aliases:
  - "r"
---
Review: $ARGUMENTS`
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "review.md"), body)

	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(project) != 1 {
		t.Fatalf("expected 1 command, got %d", len(project))
	}
	cmd := project[0]
	if cmd.Description != "review code" {
		t.Errorf("Description = %q", cmd.Description)
	}
	if len(cmd.Frontmatter.Aliases) != 1 || cmd.Frontmatter.Aliases[0] != "r" {
		t.Errorf("Aliases = %v", cmd.Frontmatter.Aliases)
	}
	if !strings.Contains(cmd.Content, "Review: $ARGUMENTS") {
		t.Errorf("Content = %q", cmd.Content)
	}
}

func TestDiscoverCommands_DescriptionFallback(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	// No frontmatter — description should fall back to first non-blank line of body.
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "review.md"), "First line description\nsecond line body")

	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(project) != 1 {
		t.Fatalf("got %d", len(project))
	}
	if project[0].Description != "First line description" {
		t.Errorf("Description = %q", project[0].Description)
	}
}
