package automemory

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// scriptedProvider returns a fixed sequence of assistant messages, then a plain
// "done" message. It records how many times Generate was called so tests can
// assert the extractor was (or was not) invoked.
type scriptedProvider struct {
	msgs      []schema.Message
	i         int
	err       error
	callCount int
}

func (p *scriptedProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.callCount++
	if p.err != nil {
		return nil, p.err
	}
	if p.i >= len(p.msgs) {
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
	}
	m := p.msgs[p.i]
	p.i++
	return &provider.GenerateResponse{Message: &m}, nil
}

func writeFileToolCall(t *testing.T, id, path, content string) schema.ToolCall {
	t.Helper()
	args, _ := json.Marshal(map[string]string{"path": path, "content": content})
	return schema.ToolCall{ID: id, Name: "write_file", Arguments: args}
}

func feedbackFile(name, desc, body string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\ntype: feedback\n---\n\n" + body + "\n"
}

func TestExtractorWritesMemoryFromSignal(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	rel, err := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "feedback-no-mock-db.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := "Do not mock the database in tests.\n\n**Why:** integration coverage matters.\n**How to apply:** use a real throwaway test DB."
	prov := &scriptedProvider{msgs: []schema.Message{
		{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{writeFileToolCall(t, "c1", rel, feedbackFile("feedback-no-mock-db", "Do not mock the DB in tests.", body))}},
		{Role: schema.RoleAssistant, Content: "saved"},
	}}

	ext := NewExtractor(prov, store, workDir)
	runMsgs := []schema.Message{{Role: schema.RoleUser, Content: "no, don't mock the database"}}
	if err := ext.Run(context.Background(), runMsgs, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	mems, _ := store.Load(ScopeProject)
	if len(mems) != 1 || mems[0].Name != "feedback-no-mock-db" {
		t.Fatalf("extraction did not write the expected memory: %+v", mems)
	}
}

func TestExtractorSkipsWhenTrackerFlagged(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)

	tracker := NewTracker(workDir, []string{store.UserGlobalDir(), store.ProjectDir()})
	// Simulate the main agent having successfully written a memory this run via
	// the workDir-relative path the file tools require.
	wroteRel, err := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "x.md"))
	if err != nil {
		t.Fatal(err)
	}
	tracker.MarkSuccess(writeCall("write_file", wroteRel), schema.ToolResult{})

	rel, _ := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "should-not-exist.md"))
	prov := &scriptedProvider{msgs: []schema.Message{
		{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{writeFileToolCall(t, "c1", rel, feedbackFile("should-not-exist", "d", "b\n\n**Why:** w\n**How to apply:** h"))}},
	}}

	ext := NewExtractor(prov, store, workDir)
	if err := ext.Run(context.Background(), nil, tracker); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if prov.callCount != 0 {
		t.Fatalf("extractor must not call the provider when the tracker is flagged (got %d calls)", prov.callCount)
	}
	mems, _ := store.Load(ScopeProject)
	if len(mems) != 0 {
		t.Fatalf("extractor must write nothing when skipped: %+v", mems)
	}
}

func TestExtractorUpdatesExistingMemoryNoDuplicate(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	if err := store.Save(Memory{Name: "feedback-tests", Description: "old desc", Type: TypeFeedback, Body: "old\n\n**Why:** w\n**How to apply:** h"}); err != nil {
		t.Fatal(err)
	}

	rel, _ := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "feedback-tests.md"))
	body := "Run the full suite before reporting done.\n\n**Why:** avoids regressions.\n**How to apply:** run go test ./... first."
	prov := &scriptedProvider{msgs: []schema.Message{
		{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{writeFileToolCall(t, "c1", rel, feedbackFile("feedback-tests", "Run full suite before done.", body))}},
		{Role: schema.RoleAssistant, Content: "updated"},
	}}

	ext := NewExtractor(prov, store, workDir)
	if err := ext.Run(context.Background(), nil, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	mems, _ := store.Load(ScopeProject)
	if len(mems) != 1 {
		t.Fatalf("dedup failed, expected 1 memory, got %d: %+v", len(mems), mems)
	}
	if mems[0].Description != "Run full suite before done." {
		t.Fatalf("existing memory was not updated: %+v", mems[0])
	}
}

func TestExtractorDoesNotMutateInputMessages(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	prov := &scriptedProvider{msgs: []schema.Message{{Role: schema.RoleAssistant, Content: "nothing to save"}}}

	runMsgs := []schema.Message{
		{Role: schema.RoleUser, Content: "hello"},
		{Role: schema.RoleAssistant, Content: "hi"},
	}
	snapshot := append([]schema.Message(nil), runMsgs...)

	ext := NewExtractor(prov, store, workDir)
	if err := ext.Run(context.Background(), runMsgs, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(runMsgs) != len(snapshot) {
		t.Fatalf("extractor mutated the input slice length: %d != %d", len(runMsgs), len(snapshot))
	}
	for i := range snapshot {
		if runMsgs[i].Content != snapshot[i].Content || runMsgs[i].Role != snapshot[i].Role {
			t.Fatalf("extractor mutated input message %d", i)
		}
	}
}

type panicProvider struct{}

func (panicProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	panic("provider exploded")
}

func TestExtractorRecoversFromPanic(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	ext := NewExtractor(panicProvider{}, store, workDir)
	if err := ext.Run(context.Background(), []schema.Message{{Role: schema.RoleUser, Content: "x"}}, nil); err != nil {
		t.Fatalf("a panicking extraction must be recovered and not propagated, got %v", err)
	}
}

func TestExtractorSwallowsProviderErrors(t *testing.T) {
	workDir := t.TempDir()
	store := NewStore(t.TempDir(), workDir)
	prov := &scriptedProvider{err: errors.New("boom")}

	ext := NewExtractor(prov, store, workDir)
	if err := ext.Run(context.Background(), []schema.Message{{Role: schema.RoleUser, Content: "x"}}, nil); err != nil {
		t.Fatalf("extraction failure must be swallowed, got %v", err)
	}
}
