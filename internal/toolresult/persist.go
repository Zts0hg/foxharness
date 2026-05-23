package toolresult

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// PersistenceThreshold is the strict-greater-than character threshold at
// which a single tool result is written to disk.
const PersistenceThreshold = 50_000

// PerTurnBudget caps the total character count of in-context tool results
// for a single turn. When the cumulative size of new results exceeds the
// budget, the largest new results are persisted to disk until the remaining
// total fits within the budget.
const PerTurnBudget = 200_000

// PreviewSize is the number of bytes from the head of a persisted result
// that remain inline in the conversation.
const PreviewSize = 2048

// FileSystem abstracts the filesystem operations required for tool result
// persistence. Production code wires this to the OS via OSFileSystem; tests
// supply in-memory implementations.
type FileSystem interface {
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
}

// OSFileSystem is the production FileSystem implementation backed by the
// standard library.
type OSFileSystem struct{}

// WriteFile delegates to os.WriteFile.
func (OSFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// Stat delegates to os.Stat.
func (OSFileSystem) Stat(path string) (os.FileInfo, error) {
	return os.Stat(path)
}

// MkdirAll delegates to os.MkdirAll.
func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// PersistedResult captures the outcome of processing a single tool result.
// Preview is the content kept inline for the model; FilePath references the
// persisted full output when Persisted is true.
type PersistedResult struct {
	Original  schema.ToolResult
	Preview   string
	FilePath  string
	Persisted bool
}

// PersistIfNeeded writes results exceeding PersistenceThreshold to disk and
// returns a Preview/FilePath description. Empty and below-threshold results
// pass through unchanged. The function is idempotent — repeated calls with
// the same ToolCallID skip writing if the file already exists, matching the
// cache-consistency guarantee from REQ-005/EC-008.
func PersistIfNeeded(fs FileSystem, dir string, result schema.ToolResult) PersistedResult {
	out := PersistedResult{Original: result, Preview: result.Output}
	if result.Output == "" {
		return out
	}
	if len(result.Output) <= PersistenceThreshold {
		return out
	}
	path := persistedFilePath(dir, result.ToolCallID)

	exists := false
	if _, err := fs.Stat(path); err == nil {
		exists = true
	}

	if !exists {
		if err := fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			out.Preview = result.Output
			return out
		}
		if err := fs.WriteFile(path, []byte(result.Output), 0o644); err != nil {
			out.Preview = result.Output
			return out
		}
	}

	out.FilePath = path
	out.Persisted = true
	out.Preview = renderPreview(result.Output, path)
	return out
}

// EnforceBudget collects the per-turn results and ensures the total inlined
// character count stays within PerTurnBudget. New results are sorted by
// in-context size descending and persisted to disk until the remaining
// in-context total fits the budget. Results whose ToolCallID appears in
// seenIDs are never retroactively persisted.
func EnforceBudget(fs FileSystem, dir string, results []PersistedResult, seenIDs map[string]bool) []PersistedResult {
	out := make([]PersistedResult, len(results))
	copy(out, results)

	total := 0
	for _, r := range out {
		total += len(r.Preview)
	}
	if total <= PerTurnBudget {
		return out
	}

	indices := make([]int, 0, len(out))
	for i, r := range out {
		if r.Persisted {
			continue
		}
		if seenIDs[r.Original.ToolCallID] {
			continue
		}
		indices = append(indices, i)
	}
	sort.SliceStable(indices, func(a, b int) bool {
		return len(out[indices[a]].Preview) > len(out[indices[b]].Preview)
	})

	for _, idx := range indices {
		if total <= PerTurnBudget {
			break
		}
		original := out[idx].Original
		persisted := PersistIfNeeded(fs, dir, original)
		if !persisted.Persisted {
			continue
		}
		total -= len(out[idx].Preview) - len(persisted.Preview)
		out[idx] = persisted
	}
	return out
}

func persistedFilePath(dir, toolCallID string) string {
	name := toolCallID
	if name == "" {
		name = "unknown"
	}
	return filepath.Join(dir, name+".txt")
}

func renderPreview(output, path string) string {
	headSize := PreviewSize
	if headSize > len(output) {
		headSize = len(output)
	}
	preview := output[:headSize]
	return fmt.Sprintf(
		"<persisted-output>\nOutput too large (%d KB). Full output saved to: %s\nPreview (first 2KB):\n%s\n</persisted-output>",
		len(output)/1024,
		path,
		preview,
	)
}
