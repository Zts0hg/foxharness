package automemory

import (
	"encoding/json"
	"os"
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
	return &Tracker{workDir: absWorkDir(workDir), memoryDirs: memoryDirs}
}

// MarkSuccess records a successful tool call: when call writes a loadable
// memory target inside a watched memory directory and result is not an error,
// it sets the wrote flag. A write_file with empty content to a direct memory
// file is treated as an explicit forget and also sets the flag. Failed results
// and other tools are ignored.
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
		if content, ok := toolCallContent(call.Arguments); call.Name == "write_file" && ok && content == "" {
			if directMemoryFileInDir(dir, resolved) {
				t.setWrote()
			}
			continue
		}
		if pathWithin(dir, resolved) {
			if t.Validator == nil || t.Validator(resolved) {
				t.setWrote()
			}
			return
		}
	}
}

func toolCallContent(raw json.RawMessage) (string, bool) {
	var args struct {
		Content *string `json:"content"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", false
	}
	if args.Content == nil {
		return "", false
	}
	return *args.Content, true
}

// WroteMemory reports whether a successful memory-directory write was observed.
func (t *Tracker) WroteMemory() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.wrote
}

func (t *Tracker) setWrote() {
	t.mu.Lock()
	t.wrote = true
	t.mu.Unlock()
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
	dir = comparablePath(dir)
	target = comparablePath(target)
	rel, err := filepath.Rel(dir, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func directMemoryFileInDir(dir, target string) bool {
	dir = comparablePath(dir)
	target = comparablePath(target)
	if filepath.Dir(target) != dir {
		return false
	}
	name := filepath.Base(target)
	return name != indexFileName && strings.HasSuffix(name, ".md")
}

func comparablePath(path string) string {
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
