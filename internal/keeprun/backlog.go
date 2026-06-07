package keeprun

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// stateFileName is the per-task pipeline progress file stored in the worktree.
const stateFileName = ".keep-run-state.json"

// Task represents a single entry in BACKLOG.md.
type Task struct {
	// Type is the category captured from the [type] heading prefix, such as
	// feature, fix, refactor, docs, chore, or test. Unknown types are preserved
	// verbatim because the type is informational (spec edge case).
	Type string
	// Title is the heading text with the [type] prefix removed.
	Title string
	// Priority is the value of the **Priority** field (high, medium, low).
	Priority string
	// Status is the value of the **Status** field (pending, done).
	Status string
	// Description is the value of the **Description** field, possibly spanning
	// multiple lines.
	Description string
	// HeadingLine is the 1-indexed line number of the "## [type]" heading. It is
	// used by UpdateStatus to rewrite the correct task's status in place.
	HeadingLine int
}

var (
	headingPattern    = regexp.MustCompile(`^##\s+\[([^\]]*)\]\s*(.*)$`)
	fieldPattern      = regexp.MustCompile(`^\s*\*\*([A-Za-z]+)\*\*\s*:\s*(.*)$`)
	statusLinePattern = regexp.MustCompile(`^(\s*\*\*[Ss]tatus\*\*\s*:\s*).*$`)
)

// ParseBacklog parses BACKLOG.md content into ordered task entries following
// the format defined in spec FR-001. Tasks are delimited by "## [type]"
// headings; the **Priority**, **Status**, and **Description** fields between two
// consecutive headings belong to the preceding task. The Description field may
// span multiple lines, continuing until the next heading or field marker.
//
// Content with no task headings yields an empty slice. ParseBacklog is tolerant
// of whitespace variations and never returns an error for well-formed markdown.
func ParseBacklog(content string) ([]Task, error) {
	lines := strings.Split(content, "\n")
	tasks := make([]Task, 0)

	current := -1
	inDescription := false
	var description strings.Builder

	flushDescription := func() {
		if current >= 0 {
			tasks[current].Description = strings.TrimSpace(description.String())
		}
		description.Reset()
		inDescription = false
	}

	for i, line := range lines {
		if m := headingPattern.FindStringSubmatch(line); m != nil {
			flushDescription()
			tasks = append(tasks, Task{
				Type:        strings.TrimSpace(m[1]),
				Title:       strings.TrimSpace(m[2]),
				HeadingLine: i + 1,
			})
			current = len(tasks) - 1
			continue
		}

		if current < 0 {
			continue
		}

		if fm := fieldPattern.FindStringSubmatch(line); fm != nil {
			if inDescription {
				flushDescription()
			}
			value := strings.TrimSpace(fm[2])
			switch strings.ToLower(fm[1]) {
			case "priority":
				tasks[current].Priority = value
			case "status":
				tasks[current].Status = value
			case "description":
				description.WriteString(value)
				inDescription = true
			}
			continue
		}

		if inDescription {
			description.WriteString("\n")
			description.WriteString(line)
		}
	}
	flushDescription()

	return tasks, nil
}

// UpdateStatus returns content with the Status field of the task whose heading
// is at headingLine (1-indexed) changed to newStatus. The marker formatting and
// all other content are preserved.
//
// If headingLine does not point at a "## [type]" heading, or no Status field is
// found before the next heading, content is returned unchanged. This makes the
// operation a safe no-op for invalid input.
func UpdateStatus(content string, headingLine int, newStatus string) string {
	lines := strings.Split(content, "\n")
	idx := headingLine - 1
	if idx < 0 || idx >= len(lines) {
		return content
	}
	if !headingPattern.MatchString(lines[idx]) {
		return content
	}

	for i := idx + 1; i < len(lines); i++ {
		if headingPattern.MatchString(lines[i]) {
			break
		}
		if m := statusLinePattern.FindStringSubmatch(lines[i]); m != nil {
			lines[i] = m[1] + newStatus
			return strings.Join(lines, "\n")
		}
	}
	return content
}

// State represents the pipeline progress for a single task, persisted as
// .keep-run-state.json inside the task's worktree. The JSON schema matches spec
// FR-002 and drives phase-level resume after interruption.
type State struct {
	TaskSlug        string `json:"task_slug"`
	WorktreePath    string `json:"worktree_path"`
	CompletedPhases []int  `json:"completed_phases"`
	RemoteEnabled   bool   `json:"remote_enabled"`
	LastPhaseAt     string `json:"last_phase_at"`
}

// ReadState reads and parses the state file from worktreeDir. A missing file is
// not an error: the zero-value State is returned so callers treat the task as
// starting from phase 1. Malformed JSON returns an error.
func ReadState(worktreeDir string) (State, error) {
	data, err := os.ReadFile(filepath.Join(worktreeDir, stateFileName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("read %s: %w", stateFileName, err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", stateFileName, err)
	}
	return state, nil
}

// WriteState writes state to .keep-run-state.json in worktreeDir, creating the
// directory if it does not yet exist. The file is written with indentation for
// human readability during debugging.
func WriteState(worktreeDir string, state State) error {
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		return fmt.Errorf("create worktree dir: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(filepath.Join(worktreeDir, stateFileName), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", stateFileName, err)
	}
	return nil
}

// NextPhase returns the 1-indexed phase number to resume from. It returns 1 when
// no phases have completed, otherwise max(CompletedPhases)+1. Using the maximum
// (rather than the first gap) means a phase is never re-run once recorded.
func (s State) NextPhase() int {
	max := 0
	for _, p := range s.CompletedPhases {
		if p > max {
			max = p
		}
	}
	return max + 1
}
