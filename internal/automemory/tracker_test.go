package automemory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
)

func writeCall(name, path string) schema.ToolCall {
	args, _ := json.Marshal(map[string]string{"path": path})
	return schema.ToolCall{ID: "c1", Name: name, Arguments: args}
}

func TestTrackerFlagsAbsoluteMemoryWrite(t *testing.T) {
	memDir := "/home/dev/.foxharness/memory"
	tr := NewTracker("/work/proj", []string{memDir})

	dec, err := tr.BeforeExecute(context.Background(), writeCall("write_file", filepath.Join(memDir, "user-role.md")))
	if err != nil {
		t.Fatalf("BeforeExecute() error = %v", err)
	}
	if dec.Type != middleware.DecisionAllow {
		t.Fatalf("tracker must observe, not block; got %v", dec.Type)
	}
	if !tr.WroteMemory() {
		t.Fatalf("WroteMemory() = false, want true after memory-dir write")
	}
}

func TestTrackerFlagsEditFileToo(t *testing.T) {
	memDir := "/home/dev/.foxharness/projects/key/memory"
	tr := NewTracker("/work/proj", []string{memDir})
	if _, err := tr.BeforeExecute(context.Background(), writeCall("edit_file", filepath.Join(memDir, "x.md"))); err != nil {
		t.Fatal(err)
	}
	if !tr.WroteMemory() {
		t.Fatalf("edit_file into memory dir must set the flag")
	}
}

func TestTrackerResolvesRelativePaths(t *testing.T) {
	workDir := "/work/proj"
	memDir := filepath.Join(workDir, "memdir")
	tr := NewTracker(workDir, []string{memDir})
	// "memdir/x.md" relative to workDir resolves inside the memory dir.
	if _, err := tr.BeforeExecute(context.Background(), writeCall("write_file", "memdir/x.md")); err != nil {
		t.Fatal(err)
	}
	if !tr.WroteMemory() {
		t.Fatalf("relative path resolving into memory dir must set the flag")
	}
}

func TestTrackerIgnoresNonMemoryWrites(t *testing.T) {
	tr := NewTracker("/work/proj", []string{"/home/dev/.foxharness/memory"})
	if _, err := tr.BeforeExecute(context.Background(), writeCall("write_file", "src/main.go")); err != nil {
		t.Fatal(err)
	}
	if tr.WroteMemory() {
		t.Fatalf("write outside the memory dir must not set the flag")
	}
}

func TestTrackerIgnoresReadsAndOtherTools(t *testing.T) {
	memDir := "/home/dev/.foxharness/memory"
	tr := NewTracker("/work/proj", []string{memDir})
	for _, name := range []string{"read_file", "bash", "subagent"} {
		if _, err := tr.BeforeExecute(context.Background(), writeCall(name, filepath.Join(memDir, "x.md"))); err != nil {
			t.Fatal(err)
		}
	}
	if tr.WroteMemory() {
		t.Fatalf("non-write tools must not set the flag")
	}
}
