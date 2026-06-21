package automemory

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func writeCall(name, path string) schema.ToolCall {
	args, _ := json.Marshal(map[string]string{"path": path})
	return schema.ToolCall{ID: "c1", Name: name, Arguments: args}
}

// TestTrackerMarksSuccessOnlyOnNonErrorMemoryWrite is the core P2-2 guarantee:
// a memory-directory write sets the flag only when the tool actually succeeded.
func TestTrackerMarksSuccessOnlyOnNonErrorMemoryWrite(t *testing.T) {
	memDir := "/home/dev/.foxharness/memory"
	tr := NewTracker("/work/proj", []string{memDir})
	call := writeCall("write_file", filepath.Join(memDir, "user-role.md"))

	tr.MarkSuccess(call, schema.ToolResult{IsError: true, Output: "edit old_string mismatch"})
	if tr.WroteMemory() {
		t.Fatalf("a failed write must not set the flag")
	}

	tr.MarkSuccess(call, schema.ToolResult{IsError: false})
	if !tr.WroteMemory() {
		t.Fatalf("a successful memory write must set the flag")
	}
}

func TestTrackerMarksSuccessForEditFile(t *testing.T) {
	memDir := "/home/dev/.foxharness/projects/key/memory"
	tr := NewTracker("/work/proj", []string{memDir})
	tr.MarkSuccess(writeCall("edit_file", filepath.Join(memDir, "x.md")), schema.ToolResult{})
	if !tr.WroteMemory() {
		t.Fatalf("a successful edit_file into the memory dir must set the flag")
	}
}

func TestTrackerResolvesRelativePaths(t *testing.T) {
	workDir := "/work/proj"
	memDir := filepath.Join(workDir, "memdir")
	tr := NewTracker(workDir, []string{memDir})
	tr.MarkSuccess(writeCall("write_file", "memdir/x.md"), schema.ToolResult{})
	if !tr.WroteMemory() {
		t.Fatalf("a relative path resolving into the memory dir must set the flag")
	}
}

func TestTrackerIgnoresNonMemoryAndNonWriteSuccess(t *testing.T) {
	memDir := "/home/dev/.foxharness/memory"
	tr := NewTracker("/work/proj", []string{memDir})

	// Successful write outside the memory dir.
	tr.MarkSuccess(writeCall("write_file", "src/main.go"), schema.ToolResult{})
	if tr.WroteMemory() {
		t.Fatalf("a write outside the memory dir must not set the flag")
	}

	// Successful read/other tool inside the memory dir.
	for _, name := range []string{"read_file", "bash", "subagent"} {
		tr.MarkSuccess(writeCall(name, filepath.Join(memDir, "x.md")), schema.ToolResult{})
	}
	if tr.WroteMemory() {
		t.Fatalf("non-write tools must not set the flag")
	}
}

func TestTrackerCallHasNoPath(t *testing.T) {
	tr := NewTracker("/work/proj", []string{"/home/dev/.foxharness/memory"})
	args, _ := json.Marshal(map[string]string{})
	tr.MarkSuccess(schema.ToolCall{ID: "c", Name: "write_file", Arguments: args}, schema.ToolResult{})
	if tr.WroteMemory() {
		t.Fatalf("a write call with no path must not set the flag")
	}
}
