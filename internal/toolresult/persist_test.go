package toolresult

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// memFS is an in-memory FileSystem implementation used by persistence tests.
type memFS struct {
	mu    sync.Mutex
	files map[string][]byte
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}}
}

func (m *memFS) WriteFile(path string, data []byte, _ os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	buf := make([]byte, len(data))
	copy(buf, data)
	m.files[path] = buf
	return nil
}

func (m *memFS) Stat(path string) (os.FileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[path]; !ok {
		return nil, os.ErrNotExist
	}
	return memFileInfo{size: int64(len(m.files[path]))}, nil
}

func (m *memFS) MkdirAll(_ string, _ os.FileMode) error {
	return nil
}

type memFileInfo struct {
	size int64
}

func (m memFileInfo) Name() string       { return "" }
func (m memFileInfo) Size() int64        { return m.size }
func (m memFileInfo) Mode() os.FileMode  { return 0 }
func (m memFileInfo) ModTime() time.Time { return time.Time{} }
func (m memFileInfo) IsDir() bool        { return false }
func (m memFileInfo) Sys() interface{}   { return nil }

func TestPersistIfNeeded_BelowThreshold(t *testing.T) {
	fs := newMemFS()
	result := schema.ToolResult{ToolCallID: "call_abc", Output: strings.Repeat("x", 30000)}
	got := PersistIfNeeded(fs, "/sess", result)
	if got.Persisted {
		t.Fatalf("PersistIfNeeded(30K) persisted = true, want false")
	}
	if got.Preview != result.Output {
		t.Fatalf("Preview should equal original output when below threshold")
	}
	if len(fs.files) != 0 {
		t.Fatalf("filesystem touched for below-threshold result: %#v", fs.files)
	}
}

func TestPersistIfNeeded_ExactlyAtThreshold(t *testing.T) {
	fs := newMemFS()
	result := schema.ToolResult{ToolCallID: "call_abc", Output: strings.Repeat("x", PersistenceThreshold)}
	got := PersistIfNeeded(fs, "/sess", result)
	if got.Persisted {
		t.Fatalf("PersistIfNeeded(exactly threshold) persisted = true, want false (threshold is strictly greater)")
	}
}

func TestPersistIfNeeded_EmptyContent(t *testing.T) {
	fs := newMemFS()
	got := PersistIfNeeded(fs, "/sess", schema.ToolResult{ToolCallID: "call_empty", Output: ""})
	if got.Persisted {
		t.Fatalf("PersistIfNeeded(empty) persisted = true, want false")
	}
	if got.Preview != "" {
		t.Fatalf("Preview = %q, want empty", got.Preview)
	}
}

func TestPersistIfNeeded_AboveThreshold(t *testing.T) {
	fs := newMemFS()
	result := schema.ToolResult{ToolCallID: "call_abc", Output: strings.Repeat("x", 60000)}
	got := PersistIfNeeded(fs, "/sess", result)
	if !got.Persisted {
		t.Fatalf("PersistIfNeeded(60K) persisted = false, want true")
	}
	if !strings.Contains(got.Preview, "<persisted-output>") {
		t.Fatalf("Preview missing <persisted-output> tag: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, "Full output saved to") {
		t.Fatalf("Preview missing file reference: %q", got.Preview)
	}
	if !strings.Contains(got.Preview, got.FilePath) {
		t.Fatalf("Preview should mention persisted file path")
	}
	if len(fs.files[got.FilePath]) != 60000 {
		t.Fatalf("disk file size = %d, want 60000", len(fs.files[got.FilePath]))
	}
}

func TestPersistIfNeeded_Idempotent(t *testing.T) {
	fs := newMemFS()
	result := schema.ToolResult{ToolCallID: "call_dup", Output: strings.Repeat("x", 60000)}
	first := PersistIfNeeded(fs, "/sess", result)
	if !first.Persisted {
		t.Fatalf("first call should persist")
	}
	originalBytes := append([]byte(nil), fs.files[first.FilePath]...)
	fs.files[first.FilePath] = []byte("tampered")
	second := PersistIfNeeded(fs, "/sess", result)
	if !second.Persisted {
		t.Fatalf("second call should still report persisted = true")
	}
	if string(fs.files[first.FilePath]) != "tampered" {
		t.Fatalf("second call should not overwrite existing file (idempotent), got %q", string(fs.files[first.FilePath]))
	}
	_ = originalBytes
}

func TestEnforceBudget_WithinBudget(t *testing.T) {
	fs := newMemFS()
	results := []PersistedResult{
		{Original: schema.ToolResult{ToolCallID: "a", Output: strings.Repeat("x", 50000)}, Preview: strings.Repeat("x", 50000)},
		{Original: schema.ToolResult{ToolCallID: "b", Output: strings.Repeat("y", 50000)}, Preview: strings.Repeat("y", 50000)},
	}
	out := EnforceBudget(fs, "/sess", results, map[string]bool{})
	for _, r := range out {
		if r.Persisted {
			t.Fatalf("EnforceBudget should not persist when within budget, got: %#v", r)
		}
	}
}

func TestEnforceBudget_OverBudgetPersistsLargestFirst(t *testing.T) {
	fs := newMemFS()
	results := []PersistedResult{
		{Original: schema.ToolResult{ToolCallID: "small", Output: strings.Repeat("a", 30000)}, Preview: strings.Repeat("a", 30000)},
		{Original: schema.ToolResult{ToolCallID: "medium", Output: strings.Repeat("b", 80000)}, Preview: strings.Repeat("b", 80000)},
		{Original: schema.ToolResult{ToolCallID: "large", Output: strings.Repeat("c", 150000)}, Preview: strings.Repeat("c", 150000)},
	}
	out := EnforceBudget(fs, "/sess", results, map[string]bool{})
	byID := map[string]PersistedResult{}
	for _, r := range out {
		byID[r.Original.ToolCallID] = r
	}
	if !byID["large"].Persisted {
		t.Fatalf("largest result (150K) should be persisted first")
	}
	if byID["small"].Persisted {
		t.Fatalf("smallest result (30K) should not be persisted when removing the largest brings total under budget")
	}
	total := 0
	for _, r := range out {
		total += len(r.Preview)
	}
	if total > PerTurnBudget {
		t.Fatalf("post-enforcement total = %d, want <= %d", total, PerTurnBudget)
	}
}

func BenchmarkPersistIfNeeded(b *testing.B) {
	dir := b.TempDir()
	content := strings.Repeat("x", 60000)
	fs := OSFileSystem{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		result := schema.ToolResult{ToolCallID: "bench-call", Output: content}
		PersistIfNeeded(fs, dir, result)
	}
}

func TestEnforceBudget_SeenResultsExcluded(t *testing.T) {
	fs := newMemFS()
	results := []PersistedResult{
		{Original: schema.ToolResult{ToolCallID: "old-call", Output: strings.Repeat("a", 150000)}, Preview: strings.Repeat("a", 150000)},
		{Original: schema.ToolResult{ToolCallID: "new-call", Output: strings.Repeat("b", 120000)}, Preview: strings.Repeat("b", 120000)},
	}
	seen := map[string]bool{"old-call": true}
	out := EnforceBudget(fs, "/sess", results, seen)
	byID := map[string]PersistedResult{}
	for _, r := range out {
		byID[r.Original.ToolCallID] = r
	}
	if byID["old-call"].Persisted {
		t.Fatalf("results in seenIDs must never be retroactively persisted")
	}
	if !byID["new-call"].Persisted {
		t.Fatalf("new-call (120K) should be persisted to bring total under budget")
	}
}
