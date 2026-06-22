package automemory

import (
	"context"
	"os"
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
	// A successful write of a valid memory into the project memory dir sets the
	// flag. The tracker validates the file, so create a real valid memory file.
	target := filepath.Join(store.ProjectDir(), "x.md")
	if err := os.MkdirAll(store.ProjectDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("---\nname: x\ndescription: d\ntype: reference\n---\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(workDir, target)
	if err != nil {
		t.Fatal(err)
	}
	tracker.MarkSuccess(writeCall("write_file", rel), schema.ToolResult{})
	if !tracker.WroteMemory() {
		t.Fatalf("tracker from PerRunHooks must flag successful valid memory writes")
	}
}

func TestPerRunHooksNormalizesRelativeWorkDirForTracker(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	workDir := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})

	store := NewStore(filepath.Join(tmp, "home"), ".")
	hooks := NewPerRunHooks(nil, store, ".")
	tracker := hooks.NewTracker()

	target := filepath.Join(store.UserGlobalDir(), "user-role.md")
	if err := os.MkdirAll(store.UserGlobalDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("---\nname: user-role\ndescription: d\ntype: user\n---\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(workDir, target)
	if err != nil {
		t.Fatal(err)
	}

	tracker.MarkSuccess(writeCall("write_file", rel), schema.ToolResult{})
	if !tracker.WroteMemory() {
		t.Fatalf("tracker from relative-workDir hooks must flag successful valid memory writes")
	}
}

// TestPerRunHooksRecordCallbackRecordsSuccessOnly proves the OnToolCalled
// callback wires the tracker to record only successful writes of valid memories.
func TestPerRunHooksRecordCallbackRecordsSuccessOnly(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	hooks := NewPerRunHooks(nil, store, workDir)
	tracker := hooks.NewTracker()
	cb := hooks.RecordCallback(tracker)

	target := filepath.Join(store.ProjectDir(), "x.md")
	if err := os.MkdirAll(store.ProjectDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("---\nname: x\ndescription: d\ntype: reference\n---\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(workDir, target)
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
		t.Fatalf("callback must record a successful valid write")
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

	// Filter by run ID: only run-2's messages reach extraction, even though
	// run-1's messages are also in the log.
	hooks.Fire(sess, "run-2", nil)

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

	var gotRunID string
	var gotTracker *Tracker
	hooks.FireFunc = func(s *session.Session, runID string, tr *Tracker) {
		gotRunID = runID
		gotTracker = tr
	}
	hooks.Fire(nil, "run-7", hooks.NewTracker())
	if gotRunID != "run-7" {
		t.Fatalf("FireFunc runID = %q, want run-7", gotRunID)
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

// TestPerRunHooksFireTrackedWaitsForCompletion proves FireTracked registers the
// launch on the provided WaitGroup so a short-lived caller can Wait for the
// extraction LLM call to finish before exiting (P2-A).
func TestPerRunHooksFireTrackedWaitsForCompletion(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewMessageLog(sess).Append("run-1", schema.Message{Role: schema.RoleUser, Content: "remember: terse answers"}); err != nil {
		t.Fatal(err)
	}

	store := NewStore(home, workDir)
	var mu sync.Mutex
	called := 0
	prov := &recordingProvider{
		onGenerate: func([]schema.Message) { mu.Lock(); called++; mu.Unlock() },
		final:      schema.Message{Role: schema.RoleAssistant, Content: "done"},
	}
	hooks := NewPerRunHooks(prov, store, workDir)

	var wg sync.WaitGroup
	hooks.FireTracked(&wg, sess, "run-1", nil)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if called == 0 {
		t.Fatalf("FireTracked must run extraction before wg.Wait() returns")
	}
}

// ctxBlockingProvider blocks every Generate until ctx is done, then returns the
// ctx error. It lets tests prove extraction honors a bounded context.
type ctxBlockingProvider struct{}

func (ctxBlockingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestPerRunHooksFireTrackedBoundedByTimeout proves FireTracked bounds the
// extraction with a timeout so a runaway/slow provider cannot hang the caller's
// Wait forever (P2-2). It shrinks the timeout for the test.
func TestPerRunHooksFireTrackedBoundedByTimeout(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewMessageLog(sess).Append("run-1", schema.Message{Role: schema.RoleUser, Content: "x"}); err != nil {
		t.Fatal(err)
	}

	store := NewStore(home, workDir)
	hooks := NewPerRunHooks(ctxBlockingProvider{}, store, workDir)

	orig := extractionTimeout
	extractionTimeout = 100 * time.Millisecond
	defer func() { extractionTimeout = orig }()

	var wg sync.WaitGroup
	start := time.Now()
	hooks.FireTracked(&wg, sess, "run-1", nil)
	wg.Wait()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("FireTracked hung %v; extraction must be bounded by its timeout", elapsed)
	}
}
