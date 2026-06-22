package agentops

import (
	"encoding/json"
	"os"
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
	registry := runner.buildRegistry(Task{ChatID: "chat"}, sess)

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

// TestAgentOpsRecordCallbackRecordsSuccessOnly proves the AgentOps runner's
// OnToolCalled wiring records only successful memory-directory writes for mutual
// exclusion (P2-2).
func TestAgentOpsRecordCallbackRecordsSuccessOnly(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	store := automemory.NewStore(home, workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)
	tracker := hooks.NewTracker()
	cb := hooks.RecordCallback(tracker)

	target := filepath.Join(store.ProjectDir(), "feedback-x.md")
	if err := os.MkdirAll(store.ProjectDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("---\nname: feedback-x\ndescription: d\ntype: reference\n---\n\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(workDir, target)
	if err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]string{"path": rel, "content": "x"})
	call := schema.ToolCall{ID: "c1", Name: "write_file", Arguments: args}

	cb(call, schema.ToolResult{IsError: true, Output: "mismatch"})
	if tracker.WroteMemory() {
		t.Fatalf("a failed write must not set the flag")
	}
	cb(call, schema.ToolResult{})
	if !tracker.WroteMemory() {
		t.Fatalf("a successful valid memory write must set the flag")
	}
}

// TestAgentOpsFireMemoryExtractionInvokesHooks proves the run-end extraction
// hook is fired with the just-finished run's seq and the tracker (P3).
func TestAgentOpsFireMemoryExtractionInvokesHooks(t *testing.T) {
	workDir := t.TempDir()
	store := automemory.NewStore(t.TempDir(), workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)

	var gotRunID string
	var gotTracker *automemory.Tracker
	hooks.FireFunc = func(s *session.Session, runID string, tr *automemory.Tracker) {
		gotRunID = runID
		gotTracker = tr
	}
	tracker := hooks.NewTracker()
	runner := &Runner{}
	runner.fireMemoryExtraction(hooks, &session.Session{ID: "s"}, "run-42", tracker)
	if gotRunID != "run-42" {
		t.Fatalf("extraction fired with runID %q, want run-42", gotRunID)
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
	hooks.FireFunc = func(*session.Session, string, *automemory.Tracker) { panic("boom") }
	runner := &Runner{}
	runner.fireMemoryExtraction(hooks, &session.Session{ID: "s"}, "", nil) // must not panic
}
