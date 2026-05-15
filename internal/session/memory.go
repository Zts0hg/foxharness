package session

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// initialWorkingMemory returns the default content for a new working memory file.
func initialWorkingMemory() string {
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

// WorkingMemory manages the agent's working memory file.
// The working memory contains the agent's current state, goals, and plans
// that persist across turns.
type WorkingMemory struct {
	// path is the file path to the working memory file.
	path string
}

// NewMemory creates a new WorkingMemory for the given session.
// Returns a WorkingMemory that operates on the session's memory file.
func NewMemory(s *Session) *WorkingMemory {
	return &WorkingMemory{path: s.MemoryPath()}
}

// Load reads and returns the current working memory content.
// Returns the full file contents, or an error if the file cannot be read.
func (m *WorkingMemory) Load() (string, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return "", fmt.Errorf("failed to read working memory: %w", err)
	}
	return string(data), nil
}

// Append adds a new note to the working memory file.
// The note is added with a timestamp and markdown formatting.
// Empty notes are ignored. Returns an error if the write fails.
func (m *WorkingMemory) Append(note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	entry := fmt.Sprintf(
		"\n## Note %s\n\n%s\n",
		time.Now().Format(time.RFC3339),
		note,
	)
	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open working memory: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(entry)
	return err
}

// Replace replaces the entire working memory content with the provided content.
// The content is trimmed of leading/trailing whitespace and a trailing newline is added.
// Returns an error if the write fails.
func (m *WorkingMemory) Replace(content string) error {
	return os.WriteFile(m.path, []byte(strings.TrimSpace(content)+"\n"), 0644)
}
