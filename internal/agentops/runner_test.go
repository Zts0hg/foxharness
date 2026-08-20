package agentops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/provider"
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

type agentOpsSurfaceProvider struct {
	mu       sync.Mutex
	surfaces [][]string
}

func (p *agentOpsSurfaceProvider) Generate(ctx context.Context, messages []schema.Message, definitions []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	p.mu.Lock()
	p.surfaces = append(p.surfaces, names)
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "analysis complete"}}, nil
}

func (p *agentOpsSurfaceProvider) firstSurface() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.surfaces) == 0 {
		return nil
	}
	return append([]string(nil), p.surfaces[0]...)
}

type recordingAgentOpsMessenger struct {
	mu    sync.Mutex
	texts []string
}

func (m *recordingAgentOpsMessenger) SendText(ctx context.Context, chatID, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	return nil
}

func (m *recordingAgentOpsMessenger) contains(substr string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, text := range m.texts {
		if strings.Contains(text, substr) {
			return true
		}
	}
	return false
}

func TestAgentOpsFirstModelCallUsesPrimaryRegistryWithoutPlannerPrepass(t *testing.T) {
	workDir := t.TempDir()
	provider := &agentOpsSurfaceProvider{}
	runner := newLegacyAgentOpsRunner(provider, workDir, t.TempDir(), &recordingAgentOpsMessenger{}, approval.NewStore())
	runner.sessions = session.NewManagerWithHome(workDir, t.TempDir())

	err := runner.run(context.Background(), Task{
		TaskID:   "task-1",
		ChatID:   "chat-1",
		SenderID: "user-1",
		Text:     "inspect the incident",
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	first := provider.firstSurface()
	if len(first) == 0 {
		t.Fatalf("first model call tools = %#v, want primary AgentOps registry without Planner prepass", first)
	}
	want := map[string]bool{"log_search": false, "read_todo": false, "update_todo": false}
	for _, name := range first {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("first model call tools = %#v, missing %s", first, name)
		}
	}
}

func TestRunnerBuildRegistryIncludesTodoTools(t *testing.T) {
	runner := &legacyAgentOpsRunner{workDir: t.TempDir()}
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

func TestRunnerStartBoundsConcurrentTasks(t *testing.T) {
	tasks := make(chan Task, 4)
	for i := 0; i < 4; i++ {
		tasks <- Task{TaskID: string(rune('a' + i)), ChatID: "chat", SenderID: "sender", Text: "task"}
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
		taskTimeout:        time.Minute,
		messenger:          &recordingAgentOpsMessenger{},
		runTask: func(ctx context.Context, task Task) error {
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
				atomic.AddInt32(&active, -1)
				return ctx.Err()
			case <-release:
				atomic.AddInt32(&active, -1)
				return nil
			}
		},
	}

	done := make(chan struct{})
	go func() {
		runner.Start(ctx, tasks)
		close(done)
	}()

	waitForAgentOpsStarts(t, started, 2)
	assertNoAdditionalAgentOpsStart(t, started)
	release <- struct{}{}
	release <- struct{}{}
	waitForAgentOpsStarts(t, started, 2)
	close(release)
	<-done

	if got := atomic.LoadInt32(&maxActive); got > 2 {
		t.Fatalf("max active tasks = %d, want <= 2", got)
	}
}

func TestRunnerRunAppliesTaskTimeout(t *testing.T) {
	messenger := &recordingAgentOpsMessenger{}
	observedDeadline := make(chan bool, 1)
	runner := &Runner{
		taskTimeout: 10 * time.Millisecond,
		messenger:   messenger,
		runTask: func(ctx context.Context, task Task) error {
			_, ok := ctx.Deadline()
			observedDeadline <- ok
			<-ctx.Done()
			return ctx.Err()
		},
	}

	runner.Run(context.Background(), Task{TaskID: "timeout", ChatID: "chat", SenderID: "sender", Text: "hang"})

	select {
	case ok := <-observedDeadline:
		if !ok {
			t.Fatalf("run task did not receive a deadline")
		}
	default:
		t.Fatalf("run task was not invoked")
	}
	if !messenger.contains("AgentOps 任务失败") {
		t.Fatalf("timeout failure was not sent to messenger: %#v", messenger.texts)
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

	runner := &legacyAgentOpsRunner{
		workDir: workDir, approvalStore: approval.NewStore(),
		newChildRunner: func(legacyChildRunnerConfig) subagent.Runner { return &nestedPermissionRunner{} },
	}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(Task{ChatID: "chat", Text: "write outside workspace"}, sess)

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
	runner := &legacyAgentOpsRunner{workDir: workDir, approvalStore: approval.NewStore()}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(Task{ChatID: "chat", Text: "inspect with shell"}, sess)

	result := registry.Execute(context.Background(), schema.ToolCall{
		ID:        "bad-bash",
		Name:      "bash",
		Arguments: mustJSON(t, map[string]string{"command": "echo $("}),
	})
	if !result.IsError || !strings.Contains(result.Output, "permission policy") {
		t.Fatalf("result = %+v, want unified permission denial", result)
	}
}

func TestRunnerBuildRegistryAllowsReadOnlyLogSearchWithoutApproval(t *testing.T) {
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "payment.log"), []byte("INFO ok\nERROR timeout\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &legacyAgentOpsRunner{workDir: t.TempDir(), logDir: logDir, approvalStore: approval.NewStore()}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(Task{ChatID: "chat", Text: "inspect logs"}, sess)

	result := registry.Execute(context.Background(), schema.ToolCall{
		ID:        "log-search",
		Name:      "log_search",
		Arguments: mustJSON(t, map[string]any{"service": "payment", "query": "timeout", "limit": 5}),
	})
	if result.IsError {
		t.Fatalf("log_search was denied or failed: %+v", result)
	}
	if !strings.Contains(result.Output, "ERROR timeout") {
		t.Fatalf("Output = %q, want matching log line", result.Output)
	}
}

func TestRunnerBuildRegistryMarksWritableDelegationNestedPermissionEnforced(t *testing.T) {
	workDir := t.TempDir()
	runner := &legacyAgentOpsRunner{
		workDir: workDir, approvalStore: approval.NewStore(),
		newChildRunner: func(legacyChildRunnerConfig) subagent.Runner { return &nestedPermissionRunner{} },
	}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(Task{ChatID: "chat", Text: "delegate a fix"}, sess)

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

	runner := &legacyAgentOpsRunner{workDir: workDir, sessions: manager}
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
	runner := &legacyAgentOpsRunner{}
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
	runner := &legacyAgentOpsRunner{}
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

func waitForAgentOpsStarts(t *testing.T, started <-chan struct{}, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for task start %d/%d", i+1, n)
		}
	}
}

func assertNoAdditionalAgentOpsStart(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
		t.Fatalf("task started despite concurrency limit")
	case <-time.After(50 * time.Millisecond):
	}
}
