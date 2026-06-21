package automemory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

// TestSC003InjectedIndexNeverExceedsLineCap covers SC-003: regardless of the
// number of memory files, the injected index for a scope is bounded.
func TestSC003InjectedIndexNeverExceedsLineCap(t *testing.T) {
	store := newTestStore(t)
	saveN(t, store, ScopeProject, 500, 0)

	merged := store.MergedIndexString()
	entries := 0
	for _, line := range strings.Split(merged, "\n") {
		if strings.HasPrefix(line, "- [") {
			entries++
		}
	}
	if entries > maxIndexLines {
		t.Fatalf("SC-003: merged index has %d entries, want <= %d", entries, maxIndexLines)
	}
	if !strings.Contains(merged, "- …") {
		t.Fatalf("SC-003: expected a truncation notice for an over-cap scope")
	}
}

// TestSC004MutualExclusionSkipsExtraction covers SC-004: a run during which the
// main agent wrote a memory produces no extraction write.
func TestSC004MutualExclusionSkipsExtraction(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	tracker := NewTracker(workDir, []string{store.UserGlobalDir(), store.ProjectDir()})
	if _, err := tracker.BeforeExecute(context.Background(), writeCall("write_file", filepath.Join(store.ProjectDir(), "x.md"))); err != nil {
		t.Fatal(err)
	}

	prov := &scriptedProvider{}
	ext := NewExtractor(prov, store, workDir)
	if err := ext.Run(context.Background(), nil, tracker); err != nil {
		t.Fatal(err)
	}
	if prov.callCount != 0 {
		t.Fatalf("SC-004: extraction must be skipped when a memory was written inline")
	}
}

// TestSC005ExtractionLeavesSessionLogUnchanged covers SC-005 / NFR-001: running
// extraction does not append anything to the session's messages.jsonl.
func TestSC005ExtractionLeavesSessionLogUnchanged(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	log := session.NewMessageLog(sess)
	if _, err := log.Append("run-1", schema.Message{Role: schema.RoleUser, Content: "no, don't mock the database"}); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append("run-1", schema.Message{Role: schema.RoleAssistant, Content: "ok"}); err != nil {
		t.Fatal(err)
	}

	before, err := os.ReadFile(sess.MessagesPath())
	if err != nil {
		t.Fatal(err)
	}

	store := NewStore(home, workDir)
	rel, _ := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "feedback-db.md"))
	file := feedbackFile("feedback-db", "Do not mock the DB.", "rule\n\n**Why:** w\n**How to apply:** h")
	prov := &scriptedProvider{msgs: []schema.Message{
		{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{writeFileToolCall(t, "c1", rel, file)}},
		{Role: schema.RoleAssistant, Content: "saved"},
	}}
	msgs, _ := log.LoadMessages()
	if err := NewExtractor(prov, store, workDir).Run(context.Background(), msgs, nil); err != nil {
		t.Fatal(err)
	}

	after, err := os.ReadFile(sess.MessagesPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("SC-005: extraction modified the session message log")
	}
	// Sanity: extraction actually ran and wrote its memory.
	if mems, _ := store.Load(ScopeProject); len(mems) != 1 {
		t.Fatalf("SC-005: expected extraction to write 1 memory, got %d", len(mems))
	}
}

// TestSC006WorkingMemoryIsSessionScopedAndNotInStore covers SC-006:
// working_memory.md is fresh per session and lives outside the automemory store.
func TestSC006WorkingMemoryIsSessionScopedAndNotInStore(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)

	s1, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	s2, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if s1.MemoryPath() == s2.MemoryPath() {
		t.Fatalf("SC-006: working_memory paths must differ per session")
	}

	store := NewStore(home, workDir)
	for _, dir := range []string{store.UserGlobalDir(), store.ProjectDir()} {
		if pathWithin(dir, s1.MemoryPath()) || pathWithin(dir, s2.MemoryPath()) {
			t.Fatalf("SC-006: working_memory must not live inside the persistent memory dir %s", dir)
		}
	}
	// The persistent store never lists working_memory content.
	if got := store.MergedIndexString(); strings.Contains(got, "working_memory") {
		t.Fatalf("SC-006: store index must never reference working_memory:\n%s", got)
	}
}

// --- Edge cases ---

func TestEdgeEmptyMemoryDirsInjectNothing(t *testing.T) {
	store := newTestStore(t)
	if got := strings.TrimSpace(store.MergedIndexString()); got != "" {
		t.Fatalf("empty dirs must inject nothing, got %q", got)
	}
	if got := strings.TrimSpace(store.Manifest()); got != "" {
		t.Fatalf("empty dirs manifest must be empty, got %q", got)
	}
}

func TestEdgeMalformedFrontmatterDoesNotBreakIndex(t *testing.T) {
	store := newTestStore(t)
	_ = store.dirs.EnsureDir(ScopeProject)
	if err := os.WriteFile(filepath.Join(store.dirs.Dir(ScopeProject), "broken.md"), []byte("garbage, no frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Memory{Name: "good", Description: "ok", Type: TypeReference, Body: "x"}); err != nil {
		t.Fatal(err)
	}
	idx, err := store.BuildIndex(ScopeProject)
	if err != nil {
		t.Fatalf("malformed file must not break index building: %v", err)
	}
	if !strings.Contains(idx, "good.md") || strings.Contains(idx, "broken.md") {
		t.Fatalf("index should list only valid memories:\n%s", idx)
	}
}

// TestEdgeExtractionCrashLeavesExistingMemoriesIntact verifies that a provider
// failure after one successful write neither corrupts existing memories nor
// leaves temp-file debris.
func TestEdgeExtractionCrashLeavesExistingMemoriesIntact(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	if err := store.Save(Memory{Name: "pre-existing", Description: "keep me", Type: TypeReference, Body: "x"}); err != nil {
		t.Fatal(err)
	}

	rel, _ := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "feedback-new.md"))
	// crashingProvider: first call writes a memory, second call errors.
	prov := &crashingProvider{
		first: schema.Message{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{writeFileToolCall(t, "c1", rel, feedbackFile("feedback-new", "new", "x\n\n**Why:** w\n**How to apply:** h"))}},
	}
	if err := NewExtractor(prov, store, workDir).Run(context.Background(), nil, nil); err != nil {
		t.Fatalf("extraction crash must be swallowed: %v", err)
	}

	mems, _ := store.Load(ScopeProject)
	names := map[string]bool{}
	for _, m := range mems {
		names[m.Name] = true
	}
	if !names["pre-existing"] {
		t.Fatalf("pre-existing memory was lost after extraction crash: %+v", mems)
	}
	// No temp-file debris in either scope dir.
	for _, scope := range []Scope{ScopeUserGlobal, ScopeProject} {
		entries, _ := os.ReadDir(store.dirs.Dir(scope))
		for _, e := range entries {
			if strings.Contains(e.Name(), ".tmp") {
				t.Fatalf("temp-file debris left behind: %s", e.Name())
			}
		}
	}
}

// crashingProvider returns one scripted assistant message, then errors on every
// subsequent call.
type crashingProvider struct {
	first schema.Message
	calls int
}

func (p *crashingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.calls++
	if p.calls == 1 {
		msg := p.first
		return &provider.GenerateResponse{Message: &msg}, nil
	}
	return nil, errors.New("provider crashed mid-extraction")
}
