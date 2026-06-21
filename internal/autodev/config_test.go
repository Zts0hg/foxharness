package autodev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, repoRoot, content string) {
	t.Helper()
	dir := filepath.Join(repoRoot, ".foxharness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir .foxharness: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "autodev.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write autodev.yml: %v", err)
	}
}

func TestLoadMissingFileAppliesDefaults(t *testing.T) {
	repoRoot := t.TempDir()

	cfg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.BacklogFile != "BACKLOG.md" {
		t.Errorf("BacklogFile = %q, want BACKLOG.md", cfg.BacklogFile)
	}
	wantWorktreeDir := filepath.Join("..", filepath.Base(repoRoot)+"-worktrees")
	if cfg.WorktreeDir != wantWorktreeDir {
		t.Errorf("WorktreeDir = %q, want %q", cfg.WorktreeDir, wantWorktreeDir)
	}
	if cfg.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want main", cfg.BaseBranch)
	}
	if cfg.Remote != "origin" {
		t.Errorf("Remote = %q, want origin", cfg.Remote)
	}
	if cfg.Concurrency != "serial" {
		t.Errorf("Concurrency = %q, want serial", cfg.Concurrency)
	}
	if cfg.Model != "" {
		t.Errorf("Model = %q, want empty (global default)", cfg.Model)
	}
	if !cfg.Gates.Build || !cfg.Gates.Test || !cfg.Gates.Gofmt {
		t.Errorf("Gates = %+v, want all true", cfg.Gates)
	}
	if !cfg.RemoteFlow.CreateIssue || !cfg.RemoteFlow.OpenPR || !cfg.RemoteFlow.LinkIssue {
		t.Errorf("RemoteFlow = %+v, want create_issue/open_pr/link_issue true", cfg.RemoteFlow)
	}
	if cfg.RemoteFlow.AutoMerge {
		t.Error("RemoteFlow.AutoMerge = true, want false by default")
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none for defaults", cfg.Warnings)
	}
}

func TestLoadFullFile(t *testing.T) {
	repoRoot := t.TempDir()
	writeConfig(t, repoRoot, `
backlog_file: TODO-LIST.md
worktree_dir: ../wt
base_branch: develop
remote: upstream
concurrency: serial
model: glm-4.7
engineer_prompt: "be terse"
engineer_prompt_file: persona.md
gates: { build: true, test: true, gofmt: true }
remote_flow:
  create_issue: true
  open_pr: true
  link_issue: true
  auto_merge: false
`)

	cfg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.BacklogFile != "TODO-LIST.md" {
		t.Errorf("BacklogFile = %q, want TODO-LIST.md", cfg.BacklogFile)
	}
	if cfg.WorktreeDir != "../wt" {
		t.Errorf("WorktreeDir = %q, want ../wt", cfg.WorktreeDir)
	}
	if cfg.BaseBranch != "develop" {
		t.Errorf("BaseBranch = %q, want develop", cfg.BaseBranch)
	}
	if cfg.Remote != "upstream" {
		t.Errorf("Remote = %q, want upstream", cfg.Remote)
	}
	if cfg.Model != "glm-4.7" {
		t.Errorf("Model = %q, want glm-4.7", cfg.Model)
	}
	if cfg.EngineerPrompt != "be terse" {
		t.Errorf("EngineerPrompt = %q, want \"be terse\"", cfg.EngineerPrompt)
	}
	if cfg.EngineerPromptFile != "persona.md" {
		t.Errorf("EngineerPromptFile = %q, want persona.md", cfg.EngineerPromptFile)
	}
}

func TestLoadPartialKeysKeepDefaultsForRest(t *testing.T) {
	repoRoot := t.TempDir()
	writeConfig(t, repoRoot, "backlog_file: OTHER.md\n")

	cfg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.BacklogFile != "OTHER.md" {
		t.Errorf("BacklogFile = %q, want OTHER.md", cfg.BacklogFile)
	}
	if cfg.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want default main", cfg.BaseBranch)
	}
	if !cfg.Gates.Test {
		t.Error("Gates.Test = false, want default true")
	}
	if cfg.RemoteFlow.AutoMerge {
		t.Error("RemoteFlow.AutoMerge = true, want default false")
	}
}

func TestLoadEmptyFileAppliesDefaults(t *testing.T) {
	repoRoot := t.TempDir()
	writeConfig(t, repoRoot, "\n")

	cfg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load returned error for empty config: %v", err)
	}
	if cfg.BacklogFile != "BACKLOG.md" {
		t.Errorf("BacklogFile = %q, want default BACKLOG.md", cfg.BacklogFile)
	}
}

func TestLoadGateFloorForcesTestOn(t *testing.T) {
	repoRoot := t.TempDir()
	writeConfig(t, repoRoot, "gates: { build: false, test: false, gofmt: false }\n")

	cfg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if !cfg.Gates.Test {
		t.Error("Gates.Test = false, want forced true (gate floor)")
	}
	if cfg.Gates.Build {
		t.Error("Gates.Build = true, want false (user disabled)")
	}
	if cfg.Gates.Gofmt {
		t.Error("Gates.Gofmt = true, want false (user disabled)")
	}
	if len(cfg.Warnings) == 0 {
		t.Fatal("Warnings empty, want prominent warnings for disabled gates")
	}
	joined := strings.Join(cfg.Warnings, "\n")
	for _, want := range []string{"test", "build", "gofmt"} {
		if !strings.Contains(strings.ToLower(joined), want) {
			t.Errorf("Warnings %q missing mention of %q gate", joined, want)
		}
	}
}

func TestLoadAutoMergeIsNeverEnabled(t *testing.T) {
	repoRoot := t.TempDir()
	writeConfig(t, repoRoot, "remote_flow: { auto_merge: true }\n")

	cfg, err := Load(repoRoot)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.RemoteFlow.AutoMerge {
		t.Error("RemoteFlow.AutoMerge = true, want forced false (REQ-020)")
	}
	if len(cfg.Warnings) == 0 {
		t.Error("Warnings empty, want a warning that auto_merge is unsupported")
	}
}

func TestLoadMalformedYAMLReturnsError(t *testing.T) {
	repoRoot := t.TempDir()
	writeConfig(t, repoRoot, "gates: [not a map\n")

	if _, err := Load(repoRoot); err == nil {
		t.Fatal("Load returned nil error for malformed YAML, want error")
	}
}

func TestLoadUnknownYAMLFieldReturnsError(t *testing.T) {
	repoRoot := t.TempDir()
	writeConfig(t, repoRoot, "pipeline: lean\n")

	if _, err := Load(repoRoot); err == nil {
		t.Fatal("Load returned nil error for removed pipeline key, want strict YAML error")
	}
}
