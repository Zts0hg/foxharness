package automemory

import (
	"encoding/json"
	"os"
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
	workDir := "/work/proj"
	memDir := workDir + "/.foxharness/memory"
	tr := NewTracker(workDir, []string{memDir})
	call := writeCall("write_file", ".foxharness/memory/user-role.md")

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
	workDir := "/work/proj"
	memDir := workDir + "/.foxharness/projects/key/memory"
	tr := NewTracker(workDir, []string{memDir})
	tr.MarkSuccess(writeCall("edit_file", ".foxharness/projects/key/memory/x.md"), schema.ToolResult{})
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

// TestTrackerAbsoluteMemoryPathMatchesFileToolResolution ensures the tracker
// resolves paths exactly like the file tools (filepath.Join(workDir, path),
// which collapses a leading slash), so an absolute memory path — which the
// tools actually write under workDir, not at the absolute target — is NOT
// classified as a memory write. Otherwise a misplaced write would falsely set
// the mutual-exclusion flag and suppress extraction (P2-2/P2-B).
func TestTrackerAbsoluteMemoryPathMatchesFileToolResolution(t *testing.T) {
	workDir := "/work/proj"
	memDir := "/home/dev/.foxharness/projects/key/memory"
	tr := NewTracker(workDir, []string{memDir})

	// The tool would write filepath.Join(workDir, "/home/.../x.md") =
	// /work/proj/home/dev/.foxharness/projects/key/memory/x.md (NOT memDir).
	tr.MarkSuccess(writeCall("write_file", filepath.Join(memDir, "x.md")), schema.ToolResult{})
	if tr.WroteMemory() {
		t.Fatalf("an absolute memory path must be classified by where the tool writes (under workDir), not flagged as a memory write")
	}

	// The intended relative form still resolves into memDir and is flagged.
	rel := relPathTo(workDir, memDir) + "/x.md"
	tr.MarkSuccess(writeCall("write_file", rel), schema.ToolResult{})
	if !tr.WroteMemory() {
		t.Fatalf("a relative path resolving into the memory dir must set the flag")
	}
}

// relPathTo returns a workDir-relative path reaching target (for constructing
// intended relative memory paths in tests).
func relPathTo(workDir, target string) string {
	rel, err := filepath.Rel(workDir, target)
	if err != nil {
		return target
	}
	return rel
}

// TestTrackerDoesNotFlagInvalidMemoryWrite proves the tracker only sets the
// mutual-exclusion flag when the write produced a valid, loadable memory — so a
// botched inline "remember" (malformed frontmatter, wrong type for the scope,
// or the index file) does not suppress the extraction backstop (P2-2).
func TestTrackerDoesNotFlagInvalidMemoryWrite(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	tr := NewTracker(workDir, []string{store.UserGlobalDir(), store.ProjectDir()})
	tr.Validator = store.IsLoadableMemoryAt

	dir := store.UserGlobalDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Invalid: malformed frontmatter.
	badPath := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(badPath, []byte("no frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	badRel, err := filepath.Rel(workDir, badPath)
	if err != nil {
		t.Fatal(err)
	}
	tr.MarkSuccess(writeCall("write_file", badRel), schema.ToolResult{})
	if tr.WroteMemory() {
		t.Fatalf("an invalid memory write must not set the flag")
	}

	// Valid: well-formed user memory.
	goodPath := filepath.Join(dir, "good.md")
	if err := os.WriteFile(goodPath, []byte("---\nname: good\ndescription: d\ntype: user\n---\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goodRel, err := filepath.Rel(workDir, goodPath)
	if err != nil {
		t.Fatal(err)
	}
	tr.MarkSuccess(writeCall("write_file", goodRel), schema.ToolResult{})
	if !tr.WroteMemory() {
		t.Fatalf("a valid memory write must set the flag")
	}
}
