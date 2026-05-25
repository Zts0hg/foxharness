package slash

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIntegrationFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestIntegration_DiscoverRegisterLookupExecute(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()

	writeIntegrationFile(t, workDir, ".foxharness/commands/review.md", `---
description: "Review code"
arguments: "scope"
---
Review the following: $scope`)

	writeIntegrationFile(t, workDir, ".foxharness/commands/db/migrate.md", `---
description: "Run migrations"
---
Migrate now`)

	writeIntegrationFile(t, workDir, ".foxharness/skills/go-test/SKILL.md", `---
description: "Run tests"
---
Run go tests`)

	writeIntegrationFile(t, userHome, ".foxharness/commands/global.md", "Global helper")

	r := NewRegistry(workDir).WithUserHome(userHome)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, name := range []string{"review", "db:migrate", "go-test", "global"} {
		cmd, ok := r.Lookup(name)
		if !ok {
			t.Errorf("Lookup(%q) failed", name)
			continue
		}
		if cmd.Type != CommandPrompt {
			t.Errorf("expected CommandPrompt for %s, got %v", name, cmd.Type)
		}
	}

	got, _ := r.Lookup("review")
	exec := NewExecutor()
	out, err := exec.Execute(context.Background(), got, "internal/engine", "sess-1")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.Content, "Review the following: internal/engine") {
		t.Errorf("output = %q", out.Content)
	}
}

func TestIntegration_ProjectOverridesUser(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeIntegrationFile(t, userHome, ".foxharness/commands/x.md", "user version")
	writeIntegrationFile(t, workDir, ".foxharness/commands/x.md", "project version")

	r := NewRegistry(workDir).WithUserHome(userHome)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	cmd, _ := r.Lookup("x")
	if cmd.Source != SourceProject {
		t.Errorf("expected SourceProject, got %v", cmd.Source)
	}
	if !strings.Contains(cmd.Content, "project version") {
		t.Errorf("content = %q", cmd.Content)
	}
}

func TestIntegration_BuiltinAndFileBasedCoexist(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	writeIntegrationFile(t, workDir, ".foxharness/commands/review.md", "review body")

	r := NewRegistry(workDir).WithUserHome(userHome)
	r.Register(&Command{Type: CommandBuiltin, Name: "help", Source: SourceBuiltin, Frontmatter: Frontmatter{UserInvocable: true}})
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	help, ok := r.Lookup("help")
	if !ok || help.Type != CommandBuiltin {
		t.Errorf("help lookup failed: %+v ok=%v", help, ok)
	}
	review, ok := r.Lookup("review")
	if !ok || review.Type != CommandPrompt {
		t.Errorf("review lookup failed: %+v ok=%v", review, ok)
	}
}

func TestIntegration_RefreshPicksUpNewFiles(t *testing.T) {
	workDir := t.TempDir()
	userHome := t.TempDir()
	commandsDir := filepath.Join(workDir, ".foxharness", "commands")
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	r := NewRegistry(workDir).WithUserHome(userHome)
	if err := r.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := r.Lookup("late"); ok {
		t.Fatal("late should not be loaded yet")
	}

	writeIntegrationFile(t, workDir, ".foxharness/commands/late.md", "late")
	if err := r.Refresh(); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if _, ok := r.Lookup("late"); !ok {
		t.Error("late should be loaded after Refresh")
	}
}
