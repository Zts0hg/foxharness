package middleware

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func guardCall(name, path string) schema.ToolCall {
	args, _ := json.Marshal(map[string]string{"path": path})
	return schema.ToolCall{ID: "c1", Name: name, Arguments: args}
}

func decide(t *testing.T, g *MemoryDirGuard, call schema.ToolCall) Decision {
	t.Helper()
	dec, err := g.BeforeExecute(context.Background(), call)
	if err != nil {
		t.Fatalf("BeforeExecute() error = %v", err)
	}
	return dec
}

func TestMemoryDirGuardAllowsWritesInsideMemoryDir(t *testing.T) {
	// The agent is instructed to address memory files by workDir-relative paths;
	// memDir lives under workDir so the relative form resolves into it.
	workDir := "/work"
	memDir := workDir + "/.foxharness/memory"
	g := NewMemoryDirGuard(workDir, []string{memDir})

	for _, name := range []string{"write_file", "edit_file"} {
		dec := decide(t, g, guardCall(name, ".foxharness/memory/x.md"))
		if dec.Type != DecisionAllow {
			t.Fatalf("%s inside memory dir = %v, want allow", name, dec.Type)
		}
	}
}

// TestMemoryDirGuardDeniesAbsoluteMemoryPath ensures an absolute memory path is
// denied: the file tools join it under workDir (not at the absolute target), so
// such a write would land outside the memory directory. The guard must classify
// by where the tool writes, matching the tools' resolution.
func TestMemoryDirGuardDeniesAbsoluteMemoryPath(t *testing.T) {
	workDir := "/work"
	memDir := "/home/dev/.foxharness/memory"
	g := NewMemoryDirGuard(workDir, []string{memDir})
	dec := decide(t, g, guardCall("write_file", filepath.Join(memDir, "x.md")))
	if dec.Type != DecisionDeny {
		t.Fatalf("absolute memory path = %v, want deny (tool would write under workDir)", dec.Type)
	}
}

func TestMemoryDirGuardDeniesWritesOutsideMemoryDir(t *testing.T) {
	g := NewMemoryDirGuard("/work", []string{"/home/dev/.foxharness/memory"})
	for _, name := range []string{"write_file", "edit_file"} {
		dec := decide(t, g, guardCall(name, "/etc/passwd"))
		if dec.Type != DecisionDeny {
			t.Fatalf("%s outside memory dir = %v, want deny", name, dec.Type)
		}
	}
	// A relative path that resolves into the project, not the memory dir, is denied.
	dec := decide(t, g, guardCall("write_file", "src/main.go"))
	if dec.Type != DecisionDeny {
		t.Fatalf("write to project src = %v, want deny", dec.Type)
	}
}

func TestMemoryDirGuardAllowsReadFileAnywhere(t *testing.T) {
	g := NewMemoryDirGuard("/work", []string{"/home/dev/.foxharness/memory"})
	dec := decide(t, g, guardCall("read_file", "src/main.go"))
	if dec.Type != DecisionAllow {
		t.Fatalf("read_file = %v, want allow (read-only)", dec.Type)
	}
}

func TestMemoryDirGuardDeniesBashAndOtherTools(t *testing.T) {
	g := NewMemoryDirGuard("/work", []string{"/home/dev/.foxharness/memory"})
	for _, name := range []string{"bash", "subagent", "some_mcp_tool"} {
		dec := decide(t, g, guardCall(name, ""))
		if dec.Type != DecisionDeny {
			t.Fatalf("%s = %v, want deny", name, dec.Type)
		}
	}
}
