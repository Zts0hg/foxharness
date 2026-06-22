package middleware

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// MemoryDirGuard narrows the extraction pass's tool permissions to the memory
// directory (REQ-013 / CON-004). Read-only file reads are allowed anywhere;
// write_file/edit_file are allowed only when their target resolves inside a
// memory directory; every other tool — including bash (which the harness cannot
// classify as read-only) and subagent/MCP/destructive tools — is denied. This is
// a more-restrictive refinement of REQ-013 (PLD-4): never less restrictive.
type MemoryDirGuard struct {
	workDir    string
	memoryDirs []string
}

// NewMemoryDirGuard constructs a guard for the given working directory and the
// set of absolute memory directories writes are confined to.
func NewMemoryDirGuard(workDir string, memoryDirs []string) *MemoryDirGuard {
	return &MemoryDirGuard{workDir: cleanAbsWorkDir(workDir), memoryDirs: memoryDirs}
}

// BeforeExecute applies the narrowing policy described on MemoryDirGuard.
func (g *MemoryDirGuard) BeforeExecute(ctx context.Context, call schema.ToolCall) (Decision, error) {
	switch call.Name {
	case "read_file":
		return Allow(), nil
	case "write_file", "edit_file":
		path := guardToolPath(call.Arguments)
		if path == "" {
			return Deny("memory extraction: write call without a path"), nil
		}
		resolved := resolveGuardPath(g.workDir, path)
		for _, dir := range g.memoryDirs {
			if guardPathWithin(dir, resolved) {
				return Allow(), nil
			}
		}
		return Deny("memory extraction may only write inside the memory directory"), nil
	default:
		return Deny("memory extraction is restricted to read-only file access and memory-directory writes"), nil
	}
}

func guardToolPath(raw json.RawMessage) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return ""
	}
	return args.Path
}

func resolveGuardPath(workDir, path string) string {
	// Mirror the file tools: they always filepath.Join(workDir, path), so an
	// absolute path is joined under workDir rather than honored as an absolute
	// target. Resolving identically keeps the guard's decision consistent with
	// where the write actually lands.
	return filepath.Join(workDir, path)
}

func cleanAbsWorkDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	if abs, err := filepath.Abs(workDir); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(workDir)
}

func guardPathWithin(dir, target string) bool {
	dir = comparableGuardPath(dir)
	target = comparableGuardPath(target)
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func comparableGuardPath(path string) string {
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}

	var missing []string
	for {
		parent := filepath.Dir(path)
		if parent == path {
			return filepath.Clean(path)
		}
		missing = append(missing, filepath.Base(path))
		path = parent
		if _, err := os.Stat(path); err == nil {
			break
		}
	}

	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = filepath.Clean(resolved)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		path = filepath.Join(path, missing[i])
	}
	return filepath.Clean(path)
}
