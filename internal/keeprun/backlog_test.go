package keeprun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const stateFileBasename = ".keep-run-state.json"

func TestParseBacklog(t *testing.T) {
	t.Run("multiple_tasks_full_fields", func(t *testing.T) {
		content := "# Backlog\n" +
			"\n" +
			"## [feature] Add dark mode\n" +
			"\n" +
			"**Priority**: high\n" +
			"**Status**: pending\n" +
			"**Description**: Add dark mode support with theme toggle and system preference detection.\n" +
			"\n" +
			"## [fix] Login timeout bug\n" +
			"\n" +
			"**Priority**: medium\n" +
			"**Status**: done\n" +
			"**Description**: Fix timeout on slow network connections when authenticating.\n" +
			"\n" +
			"## [refactor] Clean up utility functions\n" +
			"\n" +
			"**Priority**: low\n" +
			"**Status**: pending\n" +
			"**Description**: Consolidate duplicate helper functions in the utils package.\n"

		got, err := ParseBacklog(content)
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		want := []Task{
			{
				Type:        "feature",
				Title:       "Add dark mode",
				Priority:    "high",
				Status:      "pending",
				Description: "Add dark mode support with theme toggle and system preference detection.",
				HeadingLine: 3,
			},
			{
				Type:        "fix",
				Title:       "Login timeout bug",
				Priority:    "medium",
				Status:      "done",
				Description: "Fix timeout on slow network connections when authenticating.",
				HeadingLine: 9,
			},
			{
				Type:        "refactor",
				Title:       "Clean up utility functions",
				Priority:    "low",
				Status:      "pending",
				Description: "Consolidate duplicate helper functions in the utils package.",
				HeadingLine: 15,
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParseBacklog mismatch:\n got = %#v\nwant = %#v", got, want)
		}
	})

	t.Run("all_six_types", func(t *testing.T) {
		content := "## [feature] One\n**Status**: pending\n\n" +
			"## [fix] Two\n**Status**: pending\n\n" +
			"## [refactor] Three\n**Status**: pending\n\n" +
			"## [docs] Four\n**Status**: pending\n\n" +
			"## [chore] Five\n**Status**: pending\n\n" +
			"## [test] Six\n**Status**: pending\n"
		got, err := ParseBacklog(content)
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		wantTypes := []string{"feature", "fix", "refactor", "docs", "chore", "test"}
		if len(got) != len(wantTypes) {
			t.Fatalf("got %d tasks, want %d", len(got), len(wantTypes))
		}
		for i, task := range got {
			if task.Type != wantTypes[i] {
				t.Errorf("task %d Type = %q, want %q", i, task.Type, wantTypes[i])
			}
		}
	})

	t.Run("single_task", func(t *testing.T) {
		content := "## [feature] Solo task\n**Priority**: high\n**Status**: pending\n**Description**: Just one.\n"
		got, err := ParseBacklog(content)
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d tasks, want 1", len(got))
		}
		want := Task{Type: "feature", Title: "Solo task", Priority: "high", Status: "pending", Description: "Just one.", HeadingLine: 1}
		if got[0] != want {
			t.Errorf("got %#v, want %#v", got[0], want)
		}
	})

	t.Run("empty_file_returns_empty_slice", func(t *testing.T) {
		got, err := ParseBacklog("")
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d tasks, want 0", len(got))
		}
	})

	t.Run("no_tasks_only_title", func(t *testing.T) {
		got, err := ParseBacklog("# Backlog\n\nNothing here yet.\n")
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d tasks, want 0", len(got))
		}
	})

	t.Run("multi_line_description", func(t *testing.T) {
		content := "## [docs] Write guide\n" +
			"\n" +
			"**Priority**: low\n" +
			"**Status**: pending\n" +
			"**Description**: First line of description.\n" +
			"Second line continues here.\n" +
			"\n" +
			"Third paragraph after blank line.\n"
		got, err := ParseBacklog(content)
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d tasks, want 1", len(got))
		}
		wantDesc := "First line of description.\nSecond line continues here.\n\nThird paragraph after blank line."
		if got[0].Description != wantDesc {
			t.Errorf("Description = %q, want %q", got[0].Description, wantDesc)
		}
	})

	t.Run("whitespace_variations", func(t *testing.T) {
		content := "## [chore]   Spaced title\n" +
			"\n" +
			"**Priority** :   high   \n" +
			"**Status**:done\n" +
			"**Description**:   trimmed desc   \n"
		got, err := ParseBacklog(content)
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d tasks, want 1", len(got))
		}
		want := Task{Type: "chore", Title: "Spaced title", Priority: "high", Status: "done", Description: "trimmed desc", HeadingLine: 1}
		if got[0] != want {
			t.Errorf("got %#v, want %#v", got[0], want)
		}
	})

	t.Run("invalid_type_prefix_still_parsed", func(t *testing.T) {
		content := "## [bogus] Weird type\n**Status**: pending\n"
		got, err := ParseBacklog(content)
		if err != nil {
			t.Fatalf("ParseBacklog error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d tasks, want 1", len(got))
		}
		if got[0].Type != "bogus" || got[0].Title != "Weird type" {
			t.Errorf("got Type=%q Title=%q, want Type=bogus Title=Weird type", got[0].Type, got[0].Title)
		}
	})
}

func TestUpdateStatus(t *testing.T) {
	threeTask := "## [feature] One\n" +
		"**Status**: pending\n" +
		"\n" +
		"## [fix] Two\n" +
		"**Status**: pending\n" +
		"\n" +
		"## [refactor] Three\n" +
		"**Status**: pending\n"

	t.Run("pending_to_done", func(t *testing.T) {
		got := UpdateStatus("## [feature] One\n**Status**: pending\n", 1, "done")
		want := "## [feature] One\n**Status**: done\n"
		if got != want {
			t.Errorf("UpdateStatus = %q, want %q", got, want)
		}
	})

	t.Run("already_done_unchanged", func(t *testing.T) {
		content := "## [feature] One\n**Status**: done\n"
		got := UpdateStatus(content, 1, "done")
		if got != content {
			t.Errorf("UpdateStatus = %q, want unchanged %q", got, content)
		}
	})

	t.Run("invalid_heading_line_out_of_range", func(t *testing.T) {
		got := UpdateStatus(threeTask, 999, "done")
		if got != threeTask {
			t.Errorf("UpdateStatus with out-of-range line changed content")
		}
	})

	t.Run("invalid_heading_line_points_at_non_heading", func(t *testing.T) {
		// Line 2 is a status line, not a heading — must be a no-op.
		got := UpdateStatus(threeTask, 2, "done")
		if got != threeTask {
			t.Errorf("UpdateStatus pointing at non-heading changed content")
		}
	})

	t.Run("update_middle_task_preserves_others", func(t *testing.T) {
		got := UpdateStatus(threeTask, 4, "done")
		want := "## [feature] One\n" +
			"**Status**: pending\n" +
			"\n" +
			"## [fix] Two\n" +
			"**Status**: done\n" +
			"\n" +
			"## [refactor] Three\n" +
			"**Status**: pending\n"
		if got != want {
			t.Errorf("UpdateStatus middle =\n%q\nwant\n%q", got, want)
		}
	})

	t.Run("no_status_field_returns_unchanged", func(t *testing.T) {
		content := "## [feature] No status here\n**Priority**: high\n\n## [fix] Next\n**Status**: pending\n"
		got := UpdateStatus(content, 1, "done")
		if got != content {
			t.Errorf("expected unchanged when the task has no Status line, got %q", got)
		}
	})
}

func TestStateReadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	state := State{
		TaskSlug:        "add-dark-mode",
		WorktreePath:    filepath.Join(dir, ".claude", "worktrees", "add-dark-mode"),
		CompletedPhases: []int{1, 2, 3, 4, 5, 6},
		RemoteEnabled:   true,
		LastPhaseAt:     "2026-06-01T10:45:00Z",
	}
	if err := WriteState(dir, state); err != nil {
		t.Fatalf("WriteState error: %v", err)
	}
	got, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState error: %v", err)
	}
	if !reflect.DeepEqual(got, state) {
		t.Errorf("round-trip mismatch:\n got = %#v\nwant = %#v", got, state)
	}
}

func TestReadStateMissingFile(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadState(dir)
	if err != nil {
		t.Fatalf("ReadState error: %v", err)
	}
	if !reflect.DeepEqual(got, State{}) {
		t.Errorf("ReadState (missing) = %#v, want zero value", got)
	}
}

func TestReadStateInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, stateFileBasename), []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadState(dir); err == nil {
		t.Fatal("ReadState with invalid JSON: expected error, got nil")
	}
}

func TestReadStateUnreadable(t *testing.T) {
	dir := t.TempDir()
	// Make the state path a directory so ReadFile fails with a non-ErrNotExist
	// error, exercising the read-error branch.
	if err := os.Mkdir(filepath.Join(dir, stateFileBasename), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadState(dir); err == nil {
		t.Fatal("ReadState on a directory path: expected error, got nil")
	}
}

func TestWriteStateMkdirError(t *testing.T) {
	base := t.TempDir()
	filePath := filepath.Join(base, "afile")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The parent of the target dir is a regular file, so MkdirAll must fail.
	if err := WriteState(filepath.Join(filePath, "sub"), State{}); err == nil {
		t.Fatal("WriteState under a file path: expected error, got nil")
	}
}

func TestNextPhase(t *testing.T) {
	tests := []struct {
		name   string
		phases []int
		want   int
	}{
		{"empty_nil", nil, 1},
		{"empty_slice", []int{}, 1},
		{"contiguous", []int{1, 2, 3}, 4},
		{"non_contiguous_uses_max", []int{1, 3, 7}, 8},
		{"single", []int{1}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := State{CompletedPhases: tt.phases}.NextPhase()
			if got != tt.want {
				t.Errorf("NextPhase(%v) = %d, want %d", tt.phases, got, tt.want)
			}
		})
	}
}

func TestWriteStateCreatesParentDir(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "does", "not", "exist", "yet")
	if err := WriteState(nested, State{TaskSlug: "x"}); err != nil {
		t.Fatalf("WriteState error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, stateFileBasename)); err != nil {
		t.Errorf("state file not created in nested dir: %v", err)
	}
}

func TestStateFileSchema(t *testing.T) {
	dir := t.TempDir()
	state := State{
		TaskSlug:        "slug",
		WorktreePath:    "/path/to/worktree",
		CompletedPhases: []int{1, 2},
		RemoteEnabled:   true,
		LastPhaseAt:     "2026-06-01T10:45:00Z",
	}
	if err := WriteState(dir, state); err != nil {
		t.Fatalf("WriteState error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, stateFileBasename))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("state file is not valid JSON: %v", err)
	}
	for _, key := range []string{"task_slug", "worktree_path", "completed_phases", "remote_enabled", "last_phase_at"} {
		if _, ok := m[key]; !ok {
			t.Errorf("state JSON missing key %q; got keys %v", key, m)
		}
	}
	if _, ok := m["task_slug"].(string); !ok {
		t.Errorf("task_slug is not a JSON string")
	}
	if _, ok := m["remote_enabled"].(bool); !ok {
		t.Errorf("remote_enabled is not a JSON bool")
	}
	if _, ok := m["completed_phases"].([]any); !ok {
		t.Errorf("completed_phases is not a JSON array")
	}
}
