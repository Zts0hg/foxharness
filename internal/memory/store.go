package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Store manages session-local working memory and Plan Mode files.
//
// Cross-session persistent memory is no longer this package's concern: it now
// lives under ~/.foxharness via the automemory package. The legacy flat project
// MEMORY.md is neither created nor read here (REQ-017 / CON-002).
type Store struct {
	projectDir string
	sessionDir string
	now        func() time.Time
}

// NewStore creates a project-level Store. PLAN.md and TODO.md are stored in the
// project directory unless a session directory is configured with NewSessionStore.
func NewStore(workDir string) *Store {
	return &Store{projectDir: workDir, now: time.Now}
}

// NewSessionStore creates a Store that keeps PLAN.md and TODO.md in the
// session directory.
func NewSessionStore(workDir string, sessionDir string) *Store {
	return &Store{projectDir: workDir, sessionDir: sessionDir, now: time.Now}
}

// WorkingMemoryPath returns the session-scoped scratchpad path. Project-level
// stores return an empty path because working memory never belongs to a project.
func (s *Store) WorkingMemoryPath() string {
	if s.sessionDir == "" {
		return ""
	}
	return filepath.Join(s.sessionDir, "working_memory.md")
}

// EnsureWorkingMemory initializes the session scratchpad when absent. It does
// nothing for project-level stores and never replaces existing content.
func (s *Store) EnsureWorkingMemory() error {
	path := s.WorkingMemoryPath()
	if path == "" {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check working memory file %s: %w", path, err)
	}
	if err := os.WriteFile(path, []byte(workingMemoryTemplate()), 0644); err != nil {
		return fmt.Errorf("failed to initialize working memory: %w", err)
	}
	return nil
}

// LoadWorkingMemory reads the session-scoped scratchpad.
func (s *Store) LoadWorkingMemory() (string, error) {
	data, err := os.ReadFile(s.WorkingMemoryPath())
	if err != nil {
		return "", fmt.Errorf("failed to read working memory: %w", err)
	}
	return string(data), nil
}

// AppendWorkingMemory appends a timestamped note to the session scratchpad.
// Empty notes leave the file unchanged.
func (s *Store) AppendWorkingMemory(note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	entry := fmt.Sprintf("\n## Note %s\n\n%s\n", s.now().Format(time.RFC3339), note)
	f, err := os.OpenFile(s.WorkingMemoryPath(), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open working memory: %w", err)
	}
	defer f.Close()
	_, err = f.WriteString(entry)
	return err
}

// ReplaceWorkingMemory replaces the session scratchpad after trimming outer
// whitespace and preserves the historical single trailing newline.
func (s *Store) ReplaceWorkingMemory(content string) error {
	return os.WriteFile(s.WorkingMemoryPath(), []byte(strings.TrimSpace(content)+"\n"), 0644)
}

// PlanPath returns the path to the PLAN.md file.
func (s *Store) PlanPath() string {
	return filepath.Join(s.planDir(), "PLAN.md")
}

// TodoPath returns the path to the TODO.md file.
func (s *Store) TodoPath() string {
	return filepath.Join(s.planDir(), "TODO.md")
}

// EnsureFiles creates session working memory and Plan Mode files with default
// content when absent. A project-level store creates only PLAN.md and TODO.md.
// Cross-session memory remains under ~/.foxharness in the automemory package.
func (s *Store) EnsureFiles() error {
	if err := s.EnsureWorkingMemory(); err != nil {
		return err
	}
	files := map[string]string{
		s.PlanPath(): planTemplate(),
		s.TodoPath(): todoTemplate(),
	}

	for path, content := range files {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to initialize memory file %s: %w", path, err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check memory file %s: %w", path, err)
		}
	}

	return nil
}

func workingMemoryTemplate() string {
	return strings.TrimSpace(`
# Working Memory

## Goal

Not recorded.

## Known Facts

Not recorded.

## Current Plan

Not recorded.

## Next Step

Not recorded.
`) + "\n"
}

// ReplacePlan atomically replaces the session-local PLAN.md with content.
// Content is written byte-for-byte without newline or whitespace changes.
func (s *Store) ReplacePlan(content string) error {
	path := s.PlanPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create PLAN.md directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".PLAN-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary PLAN.md: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if err := tmp.Chmod(0644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to set temporary PLAN.md permissions: %w", err)
	}
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temporary PLAN.md: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temporary PLAN.md: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to replace PLAN.md: %w", err)
	}
	return nil
}

func (s *Store) planDir() string {
	if s.sessionDir != "" {
		return s.sessionDir
	}
	return s.projectDir
}

// planTemplate returns the default content for PLAN.md.
func planTemplate() string {
	return "# PLAN\n\n## Goal\n\nNot recorded.\n\n## Strategy\n\nNot recorded.\n\n## Verification\n\nNot recorded.\n"
}

// todoTemplate returns the default content for TODO.md.
func todoTemplate() string {
	return "# TODO\n\n- [ ] Not recorded.\n"
}
