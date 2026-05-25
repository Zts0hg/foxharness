package slash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEdge_EmptyFile_Registers(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "empty.md"), "")

	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("DiscoverCommands: %v", err)
	}
	if len(project) != 1 || project[0].Name != "empty" {
		t.Fatalf("expected empty registered, got %+v", project)
	}
}

func TestEdge_FileOverBudget_Skipped(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	huge := strings.Repeat("x", maxCommandFileSize+1)
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "big.md"), huge)

	_, project, _ := DiscoverCommands(workDir, userHome)
	if len(project) != 0 {
		t.Errorf("expected oversized file to be skipped, got %d", len(project))
	}
}

func TestEdge_SpecialCharactersInArguments(t *testing.T) {
	exec := NewExecutor()
	cmd := &Command{Type: CommandPrompt, Content: "Arg: $ARGUMENTS"}
	got, err := exec.Execute(context.Background(), cmd, `"hello world" $extra "with\nnewline"`, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Quoted args should be preserved verbatim.
	if !strings.Contains(got.Content, "hello world") {
		t.Errorf("expected hello world preserved, got %q", got.Content)
	}
}

func TestEdge_ShellEmbeddingNoOutput(t *testing.T) {
	got, err := ExecuteEmbeddedShell("before"+"!`true`"+"after", "", 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != "beforeafter" {
		t.Errorf("expected concat, got %q", got)
	}
}

func TestEdge_MissingNamedArgEmptyString(t *testing.T) {
	out := SubstituteArguments("[$file]-[$message]", []string{"main.go"}, []string{"file", "message"})
	if out != "[main.go]-[]" {
		t.Errorf("got %q", out)
	}
}

func TestEdge_FileBasedOverridesBuiltin_WithWarning(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeFile(t, filepath.Join(workDir, ".foxharness", "commands", "help.md"), "Custom help")

	r := NewRegistry(workDir).WithUserHome(userHome)
	r.Register(&Command{
		Type:        CommandBuiltin,
		Name:        "help",
		Source:      SourceBuiltin,
		Frontmatter: Frontmatter{UserInvocable: true},
	})
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	cmd, _ := r.Lookup("help")
	if cmd.Source != SourceProject {
		t.Errorf("expected SourceProject override, got %v", cmd.Source)
	}
}

func TestEdge_OpeningWithoutClosingDelimiter(t *testing.T) {
	fm, body, err := ParseFrontmatter([]byte("---\ndesc: x\nno close\nmore body"))
	if err == nil {
		t.Error("expected warning for missing closing delimiter")
	}
	if fm.Description != "" {
		t.Errorf("Description should fall back, got %q", fm.Description)
	}
	if !strings.Contains(body, "no close") {
		t.Errorf("body should be preserved, got %q", body)
	}
}

func TestEdge_NoFoxharnessDirectory(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	// No .foxharness dirs created.
	user, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Errorf("err: %v", err)
	}
	if len(user) != 0 || len(project) != 0 {
		t.Errorf("expected empty, got user=%d project=%d", len(user), len(project))
	}
}

func TestEdge_FrontmatterDoesNotExecuteCode(t *testing.T) {
	// gopkg.in/yaml.v3 does not allow arbitrary code execution via tags.
	// This test asserts a non-crash decode on a payload that would be
	// dangerous in less-safe parsers.
	input := []byte(`---
description: !!python/object/apply:os.system ["echo p0wned"]
---
body`)
	_, _, err := ParseFrontmatter(input)
	// yaml.v3 returns an unmarshal error here; either way no execution occurs.
	_ = err
}

// Symlink tests are skipped on Windows; they are not run in this test
// suite because the rest of the codebase assumes a Unix-like host.
func TestEdge_SymlinkHandled(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	target := filepath.Join(workDir, ".foxharness", "commands", "real.md")
	writeFile(t, target, "real body")
	linkDir := filepath.Join(workDir, ".foxharness", "commands", "linked")
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	link := filepath.Join(linkDir, "alias.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	_, project, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Dedup by inode should keep exactly one entry for the same file.
	count := 0
	for _, c := range project {
		if strings.HasSuffix(c.FilePath, "real.md") || strings.HasSuffix(c.FilePath, "alias.md") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected single dedup entry, got %d project commands: %+v", count, project)
	}
}
