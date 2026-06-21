package automemory

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// Tracker is a middleware.Middleware attached to the main run's registry that
// records whether the agent wrote to a memory directory during the run. The
// extraction hook reads this flag to enforce mutual exclusion (REQ-011): it skips
// itself when the main agent already wrote a memory. The check is a deterministic,
// path-based gate (NFR-004) — it never inspects content and never blocks a call.
type Tracker struct {
	workDir    string
	memoryDirs []string

	mu    sync.Mutex
	wrote bool
}

// NewTracker constructs a Tracker for the given working directory and the set of
// absolute memory directories to watch.
func NewTracker(workDir string, memoryDirs []string) *Tracker {
	return &Tracker{workDir: workDir, memoryDirs: memoryDirs}
}

// BeforeExecute observes write_file/edit_file calls and sets the flag when their
// target resolves inside a watched memory directory. It always returns Allow.
func (t *Tracker) BeforeExecute(ctx context.Context, call schema.ToolCall) (middleware.Decision, error) {
	switch call.Name {
	case "write_file", "edit_file":
	default:
		return middleware.Allow(), nil
	}

	path := toolCallPath(call.Arguments)
	if path == "" {
		return middleware.Allow(), nil
	}
	resolved := resolveToolPath(t.workDir, path)
	for _, dir := range t.memoryDirs {
		if pathWithin(dir, resolved) {
			t.mu.Lock()
			t.wrote = true
			t.mu.Unlock()
			break
		}
	}
	return middleware.Allow(), nil
}

// WroteMemory reports whether a memory-directory write was observed.
func (t *Tracker) WroteMemory() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.wrote
}

// toolCallPath extracts the "path" argument from a file tool call, returning ""
// when it is absent or unparseable.
func toolCallPath(raw json.RawMessage) string {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return ""
	}
	return args.Path
}

// resolveToolPath mirrors how the file tools resolve a path: relative paths are
// joined against the working directory; absolute paths are cleaned as-is.
func resolveToolPath(workDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(workDir, path))
}

// pathWithin reports whether target is the directory dir itself or lives beneath
// it, guarding against ".." escapes.
func pathWithin(dir, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
