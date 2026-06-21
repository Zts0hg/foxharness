package feishu

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestParseSessionDirective(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNew  bool
		wantText string
	}{
		{name: "plain", input: "检查日志", wantText: "检查日志"},
		{name: "slash new with prompt", input: "/new 检查日志", wantNew: true, wantText: "检查日志"},
		{name: "slash new only", input: "/new", wantNew: true, wantText: "/new"},
		{name: "chinese new", input: "新会话 修复 bug", wantNew: true, wantText: "修复 bug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNew, gotText := parseSessionDirective(tt.input)
			if gotNew != tt.wantNew {
				t.Fatalf("forceNew = %v, want %v", gotNew, tt.wantNew)
			}
			if gotText != tt.wantText {
				t.Fatalf("text = %q, want %q", gotText, tt.wantText)
			}
		})
	}
}

func TestRunnerBuildRegistryIncludesTodoTools(t *testing.T) {
	runner := &Runner{workDir: t.TempDir()}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(sess, "chat")

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

// TestFeishuBuildComposerInjectsPersistentMemory verifies the Feishu runner now
// injects the cross-session persistent memory index (REQ-006), the P3 gap Codex
// flagged.
func TestFeishuBuildComposerInjectsPersistentMemory(t *testing.T) {
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

	runner := &Runner{workDir: workDir, sessionManager: manager}
	prompt, err := runner.buildComposer(sess, store).Compose("分析日志")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Persistent Memory", "user-role.md"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("feishu composer missing %q:\n%s", want, prompt)
		}
	}
}

// TestFeishuRecordCallbackRecordsSuccessOnly proves the Feishu runner's
// OnToolCalled wiring records only successful memory-directory writes for mutual
// exclusion (P2-2).
func TestFeishuRecordCallbackRecordsSuccessOnly(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	store := automemory.NewStore(home, workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)
	tracker := hooks.NewTracker()
	cb := hooks.RecordCallback(tracker)

	rel, err := filepath.Rel(workDir, filepath.Join(store.ProjectDir(), "feedback-x.md"))
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
		t.Fatalf("a successful memory write must set the flag")
	}
}

// TestFeishuFireMemoryExtractionInvokesHooks proves the run-end extraction hook
// is fired with the just-finished run's seq and the tracker (P3).
func TestFeishuFireMemoryExtractionInvokesHooks(t *testing.T) {
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

// TestFeishuFireMemoryExtractionSwallowsPanic proves a misbehaving hook never
// disturbs the caller.
func TestFeishuFireMemoryExtractionSwallowsPanic(t *testing.T) {
	workDir := t.TempDir()
	store := automemory.NewStore(t.TempDir(), workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)
	hooks.FireFunc = func(*session.Session, int64, *automemory.Tracker) { panic("boom") }
	runner := &Runner{}
	runner.fireMemoryExtraction(hooks, &session.Session{ID: "s"}, 0, nil) // must not panic
}
