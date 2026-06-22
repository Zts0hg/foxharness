package automemory

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// Tracker records whether the main agent successfully wrote to a memory
// directory during a run. The extraction hook reads this flag to enforce mutual
// exclusion (REQ-011): it skips itself when the main agent already wrote a
// memory. The check is a deterministic, path-based gate (NFR-004).
//
// Writes are recorded only after the tool reports success (via MarkSuccess, fed
// by the engine's post-execution callback) so a failed write_file/edit_file
// never suppresses extraction for a memory the agent never actually saved.
type Tracker struct {
	workDir    string
	memoryDirs []string

	// Validator, when non-nil, is consulted on each successful memory-directory
	// write and must report whether the written path is a valid loadable memory.
	// When it returns false (e.g. malformed frontmatter, wrong type for the
	// scope, or the index file) the write does not set the flag, so the
	// extraction backstop still runs. When nil, any memory-directory write sets
	// the flag.
	Validator func(absPath string) bool

	mu    sync.Mutex
	wrote bool
}

// NewTracker constructs a Tracker for the given working directory and the set of
// absolute memory directories to watch.
func NewTracker(workDir string, memoryDirs []string) *Tracker {
	return &Tracker{workDir: workDir, memoryDirs: memoryDirs}
}

// MarkSuccess records a successful tool call: when call is a write_file or
// edit_file whose target resolves inside a watched memory directory and result
// is not an error, it sets the wrote flag. Failed results and other tools are
// ignored.
func (t *Tracker) MarkSuccess(call schema.ToolCall, result schema.ToolResult) {
	if result.IsError {
		return
	}
	switch call.Name {
	case "write_file", "edit_file":
	default:
		return
	}

	path := toolCallPath(call.Arguments)
	if path == "" {
		return
	}
	resolved := resolveToolPath(t.workDir, path)
	for _, dir := range t.memoryDirs {
		if pathWithin(dir, resolved) {
			if t.Validator == nil || t.Validator(resolved) {
				t.mu.Lock()
				t.wrote = true
				t.mu.Unlock()
			}
			return
		}
	}
}

// WroteMemory reports whether a successful memory-directory write was observed.
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

// resolveToolPath mirrors how the file tools resolve a path. The tools always do
// filepath.Join(workDir, path), which collapses a leading slash, so an absolute
// path is NOT honored as an absolute target — it is joined under workDir just
// like a relative one. Resolving identically here keeps the tracker's
// classification consistent with where the write actually lands.
func resolveToolPath(workDir, path string) string {
	return filepath.Join(workDir, path)
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
