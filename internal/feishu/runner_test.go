package feishu

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type nestedPermissionRunner struct{}

func (*nestedPermissionRunner) PermissionEnforced() bool { return true }
func (*nestedPermissionRunner) Run(context.Context, subagent.Request) (*subagent.Result, error) {
	return &subagent.Result{Status: subagent.OutcomeSucceeded}, nil
}

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

func TestRunnerBuildRegistryDeniesWorkspaceOutsideWriteWithUnifiedPermission(t *testing.T) {
	base := t.TempDir()
	workDir := filepath.Join(base, "work")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "outside.txt")
	relOutside, err := filepath.Rel(workDir, outside)
	if err != nil {
		t.Fatal(err)
	}

	runner := &Runner{
		workDir: workDir, approvalStore: approval.NewStore(),
		newChildRunner: func(ChildRunnerConfig) subagent.Runner { return &nestedPermissionRunner{} },
	}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(sess, "chat", "write outside workspace")

	result := registry.Execute(context.Background(), schema.ToolCall{
		ID:        "write-outside",
		Name:      "write_file",
		Arguments: mustJSON(t, map[string]string{"path": relOutside, "content": "blocked"}),
	})
	if !result.IsError || !strings.Contains(result.Output, "permission policy") {
		t.Fatalf("result = %+v, want unified permission denial", result)
	}
	if _, err := os.Stat(outside); !os.IsNotExist(err) {
		t.Fatalf("outside file stat error = %v, want file not written", err)
	}
}

func TestRunnerBuildRegistryDeniesUnparseableBashWithUnifiedPermission(t *testing.T) {
	workDir := t.TempDir()
	runner := &Runner{workDir: workDir, approvalStore: approval.NewStore()}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(sess, "chat", "inspect with shell")

	result := registry.Execute(context.Background(), schema.ToolCall{
		ID:        "bad-bash",
		Name:      "bash",
		Arguments: mustJSON(t, map[string]string{"command": "echo $("}),
	})
	if !result.IsError || !strings.Contains(result.Output, "permission policy") {
		t.Fatalf("result = %+v, want unified permission denial", result)
	}
}

func TestRunnerBuildRegistryMarksWritableDelegationNestedPermissionEnforced(t *testing.T) {
	workDir := t.TempDir()
	runner := &Runner{
		workDir: workDir, approvalStore: approval.NewStore(),
		newChildRunner: func(ChildRunnerConfig) subagent.Runner { return &nestedPermissionRunner{} },
	}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(sess, "chat", "delegate a fix")

	permissionRegistry, ok := registry.(tools.PermissionRegistry)
	if !ok {
		t.Fatal("registry does not expose permission assessments")
	}
	assessment, found, err := permissionRegistry.AssessPermission(
		"delegate_task",
		toolpolicy.Context{Workspace: workDir, CWD: workDir},
		mustJSON(t, map[string]any{"task": "edit the file", "read_only": false}),
	)
	if err != nil {
		t.Fatalf("AssessPermission() error = %v", err)
	}
	if !found {
		t.Fatal("delegate_task permission assessment not found")
	}
	if !assessment.NestedEnforcement || assessment.Behavior != toolpolicy.BehaviorReviewable {
		t.Fatalf("assessment = %+v, want reviewable delegation with nested enforcement", assessment)
	}
}

func TestRunnerStartBoundsConcurrentTasks(t *testing.T) {
	tasks := make(chan Task, 4)
	for i := 0; i < 4; i++ {
		id := string(rune('a' + i))
		tasks <- Task{TaskID: id, ChatID: "chat-" + id, SenderID: "sender", Text: "task"}
	}
	close(tasks)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	release := make(chan struct{})
	started := make(chan struct{}, 4)
	var active int32
	var maxActive int32

	runner := &Runner{
		maxConcurrentTasks: 2,
		runTask: func(ctx context.Context, task Task) {
			current := atomic.AddInt32(&active, 1)
			for {
				seen := atomic.LoadInt32(&maxActive)
				if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-ctx.Done():
			case <-release:
			}
			atomic.AddInt32(&active, -1)
		},
	}

	done := make(chan struct{})
	go func() {
		runner.Start(ctx, tasks)
		close(done)
	}()

	waitForStarts(t, started, 2)
	assertNoAdditionalStart(t, started)
	release <- struct{}{}
	release <- struct{}{}
	waitForStarts(t, started, 2)
	close(release)
	cancel()
	<-done

	if got := atomic.LoadInt32(&maxActive); got > 2 {
		t.Fatalf("max active tasks = %d, want <= 2", got)
	}
}

func TestRunnerSessionLocksCleanupOnlyInactiveEntries(t *testing.T) {
	now := time.Unix(1000, 0)
	runner := &Runner{
		locks:   make(map[string]*sessionLock),
		lockTTL: time.Minute,
		clock: func() time.Time {
			return now
		},
	}

	releaseActive, err := runner.acquireSessionLock(context.Background(), "active")
	if err != nil {
		t.Fatalf("acquire active session lock: %v", err)
	}
	releaseOld, err := runner.acquireSessionLock(context.Background(), "old")
	if err != nil {
		t.Fatalf("acquire old session lock: %v", err)
	}
	releaseOld()
	now = now.Add(2 * time.Minute)
	runner.cleanupSessionLocks()

	if _, ok := runner.locks["old"]; ok {
		t.Fatalf("inactive old lock was not reclaimed")
	}
	if _, ok := runner.locks["active"]; !ok {
		t.Fatalf("active lock was reclaimed")
	}
	releaseActive()
	now = now.Add(2 * time.Minute)
	runner.cleanupSessionLocks()
	if _, ok := runner.locks["active"]; ok {
		t.Fatalf("released active lock was not reclaimed after TTL")
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

// TestFeishuFireMemoryExtractionInvokesHooks proves the run-end extraction hook
// is fired with the just-finished run's seq and the tracker (P3).
func TestFeishuFireMemoryExtractionInvokesHooks(t *testing.T) {
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

// TestFeishuFireMemoryExtractionSwallowsPanic proves a misbehaving hook never
// disturbs the caller.
func TestFeishuFireMemoryExtractionSwallowsPanic(t *testing.T) {
	workDir := t.TempDir()
	store := automemory.NewStore(t.TempDir(), workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)
	hooks.FireFunc = func(*session.Session, string, *automemory.Tracker) { panic("boom") }
	runner := &Runner{}
	runner.fireMemoryExtraction(hooks, &session.Session{ID: "s"}, "", nil) // must not panic
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}

func waitForStarts(t *testing.T, started <-chan struct{}, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for task start %d/%d", i+1, n)
		}
	}
}

func assertNoAdditionalStart(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
		t.Fatalf("task started despite concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}
}
