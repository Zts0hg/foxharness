package agentops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

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

type recordingAgentOpsMessenger struct{}

func (recordingAgentOpsMessenger) SendText(ctx context.Context, chatID, text string) error {
	return nil
}

func TestAgentOpsFirstModelCallUsesPrimaryRegistryWithoutPlannerPrepass(t *testing.T) {
	workDir := t.TempDir()
	provider := &agentOpsSurfaceProvider{}
	runner := NewRunner(provider, workDir, t.TempDir(), recordingAgentOpsMessenger{}, approval.NewStore())
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

	runner := &Runner{workDir: workDir, approvalStore: approval.NewStore()}
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
	runner := &Runner{workDir: workDir, approvalStore: approval.NewStore()}
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
	runner := &Runner{workDir: t.TempDir(), logDir: logDir, approvalStore: approval.NewStore()}
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
	runner := &Runner{workDir: workDir, approvalStore: approval.NewStore()}
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

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return data
}
