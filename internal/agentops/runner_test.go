package agentops

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestRunnerBuildRegistryIncludesTodoTools(t *testing.T) {
	runner := &Runner{workDir: t.TempDir()}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(Task{ChatID: "chat"}, sess, nil)

	names := map[string]bool{}
	for _, def := range registry.GetAvailableTools() {
		names[def.Name] = true
	}
	for _, name := range []string{"read_todo", "update_todo"} {
		if !names[name] {
			t.Fatalf("registry missing %s", name)
		}
	}
}

// TestAgentOpsBuildComposerInjectsPersistentMemory verifies the AgentOps runner
// now injects the cross-session persistent memory index (REQ-006), the P3 gap
// Codex flagged.
func TestAgentOpsBuildComposerInjectsPersistentMemory(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)
	store := automemory.NewStore(home, workDir)
	if err := store.Save(automemory.Memory{
		Name:        "user-role",
		Description: "Staff engineer, terse answers.",
		Type:        automemory.TypeUser,
		Body:        "The user is a staff engineer.",
	}); err != nil {
		t.Fatal(err)
	}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir(), WorkDir: workDir}

	runner := &Runner{workDir: workDir, sessions: manager}
	prompt, err := runner.buildComposer(sess, store).Compose("分析故障")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Persistent Memory", "user-role.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("agentops composer missing %q:\n%s", want, prompt)
		}
	}
}

// TestAgentOpsBuildRegistryAttachesTracker proves the memory-write tracker is
// wired into the registry so inline memory writes are detected for mutual
// exclusion.
func TestAgentOpsBuildRegistryAttachesTracker(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	store := automemory.NewStore(home, workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)
	tracker := hooks.NewTracker()

	runner := &Runner{workDir: workDir}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir(), WorkDir: workDir}
	registry := runner.buildRegistry(Task{ChatID: "chat"}, sess, tracker)

	rel, err := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "feedback-x.md"))
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": rel, "content": "x"})
	res := registry.Execute(context.Background(), schema.ToolCall{ID: "c1", Name: "write_file", Arguments: args})
	if res.IsError {
		t.Fatalf("write_file failed: %s", res.Output)
	}
	if !tracker.WroteMemory() {
		t.Fatalf("agentops registry must detect memory-directory writes via the tracker")
	}
}

// TestAgentOpsFireMemoryExtractionInvokesHooks proves the run-end extraction
// hook is fired with the just-finished run's seq and the tracker (P3).
func TestAgentOpsFireMemoryExtractionInvokesHooks(t *testing.T) {
	workDir := t.TempDir()
	store := automemory.NewStore(t.TempDir(), workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)

	var gotSeq int64 = -1
	var gotTracker *automemory.Tracker
	hooks.FireFunc = func(s *session.Session, sinceSeq int64, tr *automemory.Tracker) {
		gotSeq = sinceSeq
		gotTracker = tr
	}
	tracker := hooks.NewTracker()
	runner := &Runner{}
	runner.fireMemoryExtraction(hooks, &session.Session{ID: "s"}, 42, tracker)
	if gotSeq != 42 {
		t.Fatalf("extraction fired with seq %d, want 42", gotSeq)
	}
	if gotTracker == nil {
		t.Fatalf("extraction must receive the tracker")
	}
}

// TestAgentOpsFireMemoryExtractionSwallowsPanic proves a misbehaving hook never
// disturbs the caller.
func TestAgentOpsFireMemoryExtractionSwallowsPanic(t *testing.T) {
	workDir := t.TempDir()
	store := automemory.NewStore(t.TempDir(), workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)
	hooks.FireFunc = func(*session.Session, int64, *automemory.Tracker) { panic("boom") }
	runner := &Runner{}
	runner.fireMemoryExtraction(hooks, &session.Session{ID: "s"}, 0, nil) // must not panic
}
