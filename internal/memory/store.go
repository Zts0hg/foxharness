package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// Store manages the session-local Plan Mode files (PLAN.md and TODO.md).
//
// Cross-session persistent memory is no longer this package's concern: it now
// lives under ~/.foxharness via the automemory package. The legacy flat project
// MEMORY.md is neither created nor read here (REQ-017 / CON-002).
type Store struct {
	projectDir string
	sessionDir string
}

// NewStore creates a project-level Store. PLAN.md and TODO.md are stored in the
// project directory unless a session directory is configured with NewSessionStore.
func NewStore(workDir string) *Store {
	return &Store{projectDir: workDir}
}

// NewSessionStore creates a Store that keeps PLAN.md and TODO.md in the
// session directory.
func NewSessionStore(workDir string, sessionDir string) *Store {
	return &Store{projectDir: workDir, sessionDir: sessionDir}
}

// PlanPath returns the path to the PLAN.md file.
func (s *Store) PlanPath() string {
	return filepath.Join(s.planDir(), "PLAN.md")
}

// TodoPath returns the path to the TODO.md file.
func (s *Store) TodoPath() string {
	return filepath.Join(s.planDir(), "TODO.md")
}

// EnsureFiles creates the session-local Plan Mode files with default content if
// they don't exist. The legacy project {workDir}/MEMORY.md is intentionally not
// created: cross-session memory now lives under ~/.foxharness via the automemory
// package (REQ-017 / CON-002). Returns an error if any file cannot be created.
func (s *Store) EnsureFiles() error {
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
