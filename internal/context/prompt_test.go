package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComposeDoesNotAutoLoadSkills(t *testing.T) {
	workDir := t.TempDir()
	skillDir := filepath.Join(workDir, ".foxharness", "skills", "go-refactor")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Should never load"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).Compose()
	if err != nil {
		t.Fatalf("Compose returned error: %v", err)
	}
	for _, unwanted := range []string{"Loaded Skill", "Requested as:", "Should never load"} {
		if strings.Contains(prompt, unwanted) {
			t.Fatalf("Compose must not auto-load skills; found %q:\n%s", unwanted, prompt)
		}
	}
}

func TestComposeLoadsProjectMemorySeparatelyFromWorkingMemory(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(workDir, "MEMORY.md"), []byte("project convention"), 0o644); err != nil {
		t.Fatal(err)
	}
	workingMemoryPath := filepath.Join(sessionDir, "working_memory.md")
	if err := os.WriteFile(workingMemoryPath, []byte("session note"), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := NewComposer(workDir).WithMemory(workingMemoryPath).Compose()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## Project Memory from MEMORY.md",
		"project convention",
		"## Session Working Memory",
		"session note",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestComposeIncludesTodoToolInstructions(t *testing.T) {
	prompt, err := NewComposer(t.TempDir()).Compose()
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
