package memory

import (
	"fmt"
	"os"
	"path/filepath"
)

// Store manages the memory files for Plan Mode (PLAN.md, TODO.md, MEMORY.md).
// These files provide persistent storage for planning-related state.
type Store struct {
	// workDir is the directory where memory files are stored.
	workDir string
}

// NewStore creates a new Store for the given working directory.
// Returns a Store that manages memory files in the specified directory.
func NewStore(workDir string) *Store {
	return &Store{workDir: workDir}
}

// PlanPath returns the path to the PLAN.md file.
func (s *Store) PlanPath() string {
	return filepath.Join(s.workDir, "PLAN.md")
}

// TodoPath returns the path to the TODO.md file.
func (s *Store) TodoPath() string {
	return filepath.Join(s.workDir, "TODO.md")
}

// MemoryPath returns the path to the MEMORY.md file.
func (s *Store) MemoryPath() string {
	return filepath.Join(s.workDir, "MEMORY.md")
}

// EnsureFiles creates memory files with default content if they don't exist.
// Returns an error if any file cannot be created.
func (s *Store) EnsureFiles() error {
	files := map[string]string{
		s.PlanPath():   planTemplate(),
		s.TodoPath():   todoTemplate(),
		s.MemoryPath(): memoryTemplate(),
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

// planTemplate returns the default content for PLAN.md.
func planTemplate() string {
	return "# PLAN\n\n## Goal\n\nNot recorded.\n\n## Strategy\n\nNot recorded.\n\n## Verification\n\nNot recorded.\n"
}

// todoTemplate returns the default content for TODO.md.
func todoTemplate() string {
	return "# TODO\n\n- [ ] Not recorded.\n"
}

// memoryTemplate returns the default content for MEMORY.md.
func memoryTemplate() string {
	return "# MEMORY\n\n- Not recorded.\n"
}

// Bundle contains the contents of all memory files.
type Bundle struct {
	// Plan is the content of PLAN.md.
	Plan string
	// Todo is the content of TODO.md.
	Todo string
	// Memory is the content of MEMORY.md.
	Memory string
}

// Load reads all memory files and returns their contents.
// Missing files are treated as empty. Returns an error if reading fails.
func (s *Store) Load() (*Bundle, error) {
	plan, err := readOptional(s.PlanPath())
	if err != nil {
		return nil, err
	}
	todo, err := readOptional(s.TodoPath())
	if err != nil {
		return nil, err
	}
	mem, err := readOptional(s.MemoryPath())
	if err != nil {
		return nil, err
	}

	return &Bundle{
		Plan:   plan,
		Todo:   todo,
		Memory: mem,
	}, nil
}

// readOptional reads a file, returning empty string if it doesn't exist.
func readOptional(path string) (string, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return string(data), nil
}
