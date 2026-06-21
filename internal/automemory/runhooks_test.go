package automemory

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestPerRunHooksNewTrackerWatchesMemoryDirs(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	hooks := NewPerRunHooks(nil, store, workDir)

	tracker := hooks.NewTracker()
	if tracker == nil {
		t.Fatalf("NewTracker() returned nil")
	}
	// A successful write into the project memory dir sets the flag.
	rel, err := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "x.md"))
	if err != nil {
		t.Fatal(err)
	}
	tracker.MarkSuccess(writeCall("write_file", rel), schema.ToolResult{})
	if !tracker.WroteMemory() {
		t.Fatalf("tracker from PerRunHooks must flag successful memory writes")
	}
}

// TestPerRunHooksRecordCallbackRecordsSuccessOnly proves the OnToolCalled
// callback wires the tracker to record only successful writes.
func TestPerRunHooksRecordCallbackRecordsSuccessOnly(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	hooks := NewPerRunHooks(nil, store, workDir)
	tracker := hooks.NewTracker()
	cb := hooks.RecordCallback(tracker)

	rel, err := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "x.md"))
	if err != nil {
		t.Fatal(err)
	}
	call := writeCall("write_file", rel)

	cb(call, schema.ToolResult{IsError: true, Output: "mismatch"})
	if tracker.WroteMemory() {
		t.Fatalf("callback must not record a failed write")
	}
	cb(call, schema.ToolResult{})
	if !tracker.WroteMemory() {
		t.Fatalf("callback must record a successful write")
	}
}

func TestPerRunHooksFireFiltersMessagesToRunAndCallsProvider(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	log := session.NewMessageLog(sess)

	// run-1: a prior, already-forgotten signal that must NOT reach extraction.
	if _, err := log.Append("run-1", schema.Message{Role: schema.RoleUser, Content: "please forget my old preference"}); err != nil {
		t.Fatal(err)
	}
	if _, err := log.Append("run-1", schema.Message{Role: schema.RoleAssistant, Content: "ok forgotten"}); err != nil {
		t.Fatal(err)
	}
	run2Start, err := log.NextSeq()
	if err != nil {
		t.Fatal(err)
	}
	// run-2: the just-finished run carrying a saveable signal.
	if _, err := log.Append("run-2", schema.Message{Role: schema.RoleUser, Content: "no, do not mock the database"}); err != nil {
		t.Fatal(err)
	}

	store := NewStore(home, workDir)
	var mu sync.Mutex
	var rendered string
	prov := &recordingProvider{
		onGenerate: func(msgs []schema.Message) {
			for _, m := range msgs {
				if m.Role == schema.RoleUser && contains(m.Content, "Conversation to review") {
					mu.Lock()
					rendered = m.Content
					mu.Unlock()
					return
				}
			}
		},
		final: schema.Message{Role: schema.RoleAssistant, Content: "nothing to save"},
	}
	hooks := NewPerRunHooks(prov, store, workDir)

	hooks.Fire(sess, run2Start, nil)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		done := rendered != ""
		mu.Unlock()
		if done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if rendered == "" {
		t.Fatalf("extraction was not invoked")
	}
	if !contains(rendered, "do not mock the database") {
		t.Fatalf("extraction must see the current run's signal:\n%s", rendered)
	}
	if contains(rendered, "forget my old preference") {
		t.Fatalf("extraction must NOT reprocess prior runs:\n%s", rendered)
	}
}

func TestPerRunHooksFireFuncOverride(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	hooks := NewPerRunHooks(nil, store, workDir)

	var gotSeq int64 = -1
	var gotTracker *Tracker
	hooks.FireFunc = func(s *session.Session, sinceSeq int64, tr *Tracker) {
		gotSeq = sinceSeq
		gotTracker = tr
	}
	hooks.Fire(nil, 7, hooks.NewTracker())
	if gotSeq != 7 {
		t.Fatalf("FireFunc sinceSeq = %d, want 7", gotSeq)
	}
	if gotTracker == nil {
		t.Fatalf("FireFunc must receive the tracker")
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return len(needle) == 0
}

// recordingProvider invokes onGenerate for each Generate call and returns final.
type recordingProvider struct {
	onGenerate func(msgs []schema.Message)
	final      schema.Message
}

func (p *recordingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	if p.onGenerate != nil {
		p.onGenerate(messages)
	}
	msg := p.final
	return &provider.GenerateResponse{Message: &msg}, nil
}
