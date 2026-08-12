package feishu

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPFFEI001CurrentProfileSnapshotIsFlatAndRestrictionsOnlyNarrow(t *testing.T) {
	runner, sess := newFeishuProfileRunner(t, &feishuProfileProvider{})
	registry := runner.buildRegistry(sess, "chat", "current prompt")
	got := currentFeishuProfileSnapshot{
		name: "FeishuRemote", sessionSource: string(sess.Source), workspace: runner.workDir,
		maxTurns: 20, taskTimeout: runner.taskTimeoutOrDefault(), maxConcurrent: runner.concurrentTaskLimit(),
		modelScope: "provider-fixed", thinking: false, streaming: false,
		permission: "ask", checkpoint: false, rewind: false, skill: false, formalPlan: false,
		memory: true, automaticCompaction: true, extraction: "fire-and-forget", observation: "facts-no-deltas",
		canonicalTools: feishuProfileToolNames(registry.GetAvailableTools()),
	}
	want := currentFeishuProfileSnapshot{
		name: "FeishuRemote", sessionSource: string(session.SOURCEFeishu), workspace: runner.workDir,
		maxTurns: 20, taskTimeout: 5 * time.Minute, maxConcurrent: 4,
		modelScope: "provider-fixed", permission: "ask", memory: true, automaticCompaction: true,
		extraction: "fire-and-forget", observation: "facts-no-deltas",
		canonicalTools: "bash,delegate_task,edit_file,read_file,read_todo,update_todo,write_file",
	}
	if got != want {
		t.Fatalf("current Feishu profile = %#v, want %#v", got, want)
	}
	filtered := tools.NewFilteredRegistry(registry, []string{"read_file", "not-in-profile"})
	if names := feishuProfileToolNames(filtered.GetAvailableTools()); names != "read_file" {
		t.Fatalf("restricted tools = %q, want read_file", names)
	}
	if result := filtered.Execute(context.Background(), schema.ToolCall{ID: "expand", Name: "not-in-profile"}); !result.IsError {
		t.Fatalf("restriction expanded profile: %#v", result)
	}
}

func TestPFFEI002TaskIdentityAndSourceEnvelopeAreExact(t *testing.T) {
	event := messageEvent("event-1", "message-1", true)
	content := "  inspect logs  "
	event.Event.Message.Content = &content
	task, err := taskFromMessageEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID == "" || task.ChatID != "chat-1" || task.SenderID != "sender-1" || task.MessageID != "message-1" || task.Text != "inspect logs" {
		t.Fatalf("task identity = %#v", task)
	}
	promptText := feishuTaskPrompt(task, task.Text)
	want := "以下任务来自飞书用户 sender-1，消息 ID 为 message-1。\n\ninspect logs"
	if promptText != want {
		t.Fatalf("source envelope = %q, want %q", promptText, want)
	}
	forceNew, text := parseSessionDirective(" /new inspect logs ")
	if !forceNew || text != "inspect logs" {
		t.Fatalf("remote /new directive = %v/%q", forceNew, text)
	}
}

func TestPFFEI003SessionDirectiveAndSelectionMatrix(t *testing.T) {
	for _, tc := range []struct {
		input    string
		wantNew  bool
		wantText string
	}{
		{input: " task ", wantText: "task"},
		{input: "/new", wantNew: true, wantText: "/new"},
		{input: " /new next ", wantNew: true, wantText: "next"},
		{input: "新会话", wantNew: true, wantText: "新会话"},
		{input: " 新会话 next ", wantNew: true, wantText: "next"},
		{input: "/newer task", wantText: "/newer task"},
		{input: "新会话题", wantText: "新会话题"},
	} {
		gotNew, gotText := parseSessionDirective(tc.input)
		if gotNew != tc.wantNew || gotText != tc.wantText {
			t.Errorf("parseSessionDirective(%q) = %v/%q, want %v/%q", tc.input, gotNew, gotText, tc.wantNew, tc.wantText)
		}
	}

	runner, first := newFeishuProfileRunner(t, &feishuProfileProvider{})
	task := Task{ChatID: "chat", SenderID: "sender"}
	continued, created, err := runner.resolveSession(false, task)
	if err != nil || created || continued.ID != first.ID {
		t.Fatalf("continued session = %#v/%v/%v", continued, created, err)
	}
	forced, created, err := runner.resolveSession(true, task)
	if err != nil || !created || forced.ID == first.ID {
		t.Fatalf("forced session = %#v/%v/%v", forced, created, err)
	}
	other, created, err := runner.resolveSession(false, Task{ChatID: "other", SenderID: "sender"})
	if err != nil || !created || other.ID == first.ID || other.ID == forced.ID {
		t.Fatalf("isolated session = %#v/%v/%v", other, created, err)
	}
}

func TestPFFEI004SessionConversationAndArtifactsStayIsolated(t *testing.T) {
	runner, first := newFeishuProfileRunner(t, &feishuProfileProvider{})
	firstLog := session.NewMessageLog(first)
	if _, err := firstLog.Append("run-1", schema.Message{Role: schema.RoleUser, Content: "first task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := firstLog.Append("run-1", schema.Message{Role: schema.RoleAssistant, Content: "first answer"}); err != nil {
		t.Fatal(err)
	}
	continued, _, err := runner.resolveSession(false, Task{ChatID: first.ChatID, SenderID: first.UserID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := session.NewMessageLog(continued).Append("run-2", schema.Message{Role: schema.RoleUser, Content: "second task"}); err != nil {
		t.Fatal(err)
	}
	records, err := session.NewMessageLog(continued).LoadRecords()
	if err != nil || len(records) != 3 || records[0].RunID != "run-1" || records[2].RunID != "run-2" {
		t.Fatalf("continued records = %#v, %v", records, err)
	}
	isolated, _, err := runner.resolveSession(false, Task{ChatID: "other-chat", SenderID: first.UserID})
	if err != nil {
		t.Fatal(err)
	}
	if isolated.ID == first.ID || isolated.RootDir == first.RootDir {
		t.Fatalf("isolated session leaked identity/artifacts: %#v/%#v", first, isolated)
	}
	if isolatedRecords, err := session.NewMessageLog(isolated).LoadRecords(); err != nil || len(isolatedRecords) != 0 {
		t.Fatalf("isolated records = %#v, %v", isolatedRecords, err)
	}
}

func TestPFFEI006And007ExactRemoteCapabilitySurface(t *testing.T) {
	runner, sess := newFeishuProfileRunner(t, &feishuProfileProvider{})
	registry := runner.buildRegistry(sess, "chat", "task")
	if got, want := feishuProfileToolNames(registry.GetAvailableTools()), "bash,delegate_task,edit_file,read_file,read_todo,update_todo,write_file"; got != want {
		t.Fatalf("tool surface = %q, want %q", got, want)
	}
	for _, forbidden := range []string{"skill", "ask_user_question", "AskUserQuestion", "submit_plan"} {
		if feishuRegistryHasTool(registry, forbidden) {
			t.Fatalf("remote profile exposed %q", forbidden)
		}
	}
	permissionRegistry, ok := registry.(tools.PermissionRegistry)
	if !ok {
		t.Fatalf("registry %T lacks permission assessment", registry)
	}
	assessment, found, err := permissionRegistry.AssessPermission("read_file", toolpolicy.Context{Workspace: runner.workDir, CWD: runner.workDir}, []byte(`{"path":"fixture.txt"}`))
	if err != nil || !found || assessment.Behavior != toolpolicy.BehaviorFastAllow {
		t.Fatalf("read permission = %#v/%v/%v", assessment, found, err)
	}
	for _, definition := range registry.GetAvailableTools() {
		if registry.IsParallelSafe(definition.Name) {
			t.Fatalf("decorated remote tool %q unexpectedly parallel-safe", definition.Name)
		}
	}
	if err := os.WriteFile(filepath.Join(runner.workDir, "fixture.txt"), []byte("remote fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := registry.Execute(context.Background(), schema.ToolCall{ID: "read", Name: "read_file", Arguments: []byte(`{"path":"fixture.txt"}`)})
	if result.IsError || !strings.Contains(result.Output, "remote fixture") {
		t.Fatalf("read execution = %#v", result)
	}
}

func TestPFFEI009ApprovalEvidenceIsSessionBoundedAndDeduplicated(t *testing.T) {
	_, sess := newFeishuProfileRunner(t, &feishuProfileProvider{})
	current := "以下任务来自飞书用户 sender，消息 ID 为 message。\n\ninspect"
	if _, err := session.NewMessageLog(sess).Append("prior", schema.Message{Role: schema.RoleUser, Content: "prior task"}); err != nil {
		t.Fatal(err)
	}
	provider := remotePermissionEvidenceProvider(sess, current)
	request := permissionRequestForProfile("write_file", `{"path":"x","content":"y"}`)
	evidence := provider(request)
	joined := evidence.Text
	if strings.Count(joined, "prior task") != 1 || strings.Count(joined, current) != 1 {
		t.Fatalf("approval evidence duplicated/leaked current session:\n%s", joined)
	}
	if !strings.Contains(evidence.Trusted, `"tool":"write_file"`) || !strings.Contains(evidence.Trusted, `"action":"write"`) {
		t.Fatalf("approval request identity = %#v", evidence)
	}
}

func TestPFFEI011ContextCompositionHasRemoteFragmentsOnly(t *testing.T) {
	runner, sess := newFeishuProfileRunner(t, &feishuProfileProvider{})
	if err := os.WriteFile(filepath.Join(runner.workDir, "AGENTS.md"), []byte("FEISHU_PROJECT_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sess.MemoryPath(), []byte("FEISHU_SESSION_MEMORY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := automemory.NewStore(runner.sessionManager.HomeDir(), runner.workDir)
	if err := store.Save(automemory.Memory{Name: "feishu-profile", Description: "remote memory", Type: automemory.TypeProject, Body: "Remote fact.\n\n**Why:** test.\n**How to apply:** inject."}); err != nil {
		t.Fatal(err)
	}
	composed, err := runner.buildComposer(sess, store).Compose("remote task")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"FEISHU_PROJECT_INSTRUCTION", "FEISHU_SESSION_MEMORY", "feishu-profile.md"} {
		if strings.Count(composed, fragment) != 1 {
			t.Fatalf("prompt contains %q %d times:\n%s", fragment, strings.Count(composed, fragment), composed)
		}
	}
	for _, forbidden := range []string{"Available Skills", "Collaboration Mode", "ask_user_question", "Formal Plan"} {
		if strings.Contains(composed, forbidden) {
			t.Fatalf("remote prompt contains TUI/skill fragment %q:\n%s", forbidden, composed)
		}
	}
	if _, err := os.Stat(filepath.Join(sess.RootDir, "checkpoints.jsonl")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remote composition created checkpoint state: %v", err)
	}
}

func TestPFFEI013ExtractionIsRunBoundedFireAndForget(t *testing.T) {
	workDir := t.TempDir()
	store := automemory.NewStore(t.TempDir(), workDir)
	modelProvider := &feishuExtractionBarrierProvider{started: make(chan struct{}), release: make(chan struct{})}
	hooks := automemory.NewPerRunHooks(modelProvider, store, workDir)
	sess := &session.Session{ID: "s", RootDir: t.TempDir()}
	if _, err := session.NewMessageLog(sess).Append("run-42", schema.Message{Role: schema.RoleUser, Content: "extract this run"}); err != nil {
		t.Fatal(err)
	}
	(&Runner{}).fireMemoryExtraction(hooks, sess, "run-42", hooks.NewTracker())
	select {
	case <-modelProvider.started:
	case <-time.After(time.Second):
		t.Fatal("extraction did not start")
	}
	// Reaching this point while Generate is blocked proves Fire returned first.
	close(modelProvider.release)
}

func TestPFFEI015ReporterDoesNotRequestStreamingOrThinking(t *testing.T) {
	reporter := NewReporter(nil, "chat", "task")
	if _, ok := any(reporter).(engine.MessageDeltaReporter); ok {
		t.Fatal("Feishu reporter exposes model-delta observation")
	}
	source := readPackageSource(t, "runner.go")
	for _, required := range []string{"EnableThinking: false", "MaxTurns:       20"} {
		if !strings.Contains(source, required) {
			t.Fatalf("runner source missing frozen config %q", required)
		}
	}
}

func TestPFFEI018CoreDoesNotDependOnLarkOrFeishuPresentation(t *testing.T) {
	for _, dir := range []string{filepath.Join("..", "engine"), filepath.Join("..", "app")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{"larksuite", "internal/feishu", "/webhook/", "飞书", "任务已进入新 Session"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatalf("core file %s contains transport token %q", filepath.Join(dir, entry.Name()), forbidden)
				}
			}
		}
	}
	for _, name := range []string{"gateway.go", "messenger.go", "reporter.go", "runner.go"} {
		if _, err := os.Stat(name); err != nil {
			t.Fatalf("current Feishu adapter ownership missing %s: %v", name, err)
		}
	}
}

type currentFeishuProfileSnapshot struct {
	name, sessionSource, workspace, modelScope, permission, extraction, observation, canonicalTools string
	maxTurns, maxConcurrent                                                                         int
	taskTimeout                                                                                     time.Duration
	thinking, streaming, checkpoint, rewind, skill, formalPlan, memory, automaticCompaction         bool
}

type feishuProfileProvider struct {
	mu           sync.Mutex
	observations [][]schema.Message
	streamCalls  atomic.Int32
}

type feishuExtractionBarrierProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *feishuExtractionBarrierProvider) Generate(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (p *feishuProfileProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.observations = append(p.observations, append([]schema.Message(nil), messages...))
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (*feishuProfileProvider) ProviderProtocol() string { return "scripted" }
func (*feishuProfileProvider) ModelName() string        { return "claude-4-sonnet" }

func newFeishuProfileRunner(t *testing.T, modelProvider provider.LLMProvider) (*Runner, *session.Session) {
	t.Helper()
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCEFeishu, WorkDir: workDir, UserID: "sender", ChatID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	return NewRunner(modelProvider, workDir, nil, manager, approval.NewStore()), sess
}

func feishuProfileToolNames(definitions []schema.ToolDefinition) string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func feishuRegistryHasTool(registry tools.Registry, name string) bool {
	for _, definition := range registry.GetAvailableTools() {
		if definition.Name == name {
			return true
		}
	}
	return false
}

func permissionRequestForProfile(toolName, arguments string) permission.Request {
	return permission.Request{ToolName: toolName, Arguments: arguments, Action: "write", ToolCall: schema.ToolCall{Name: toolName, Arguments: []byte(arguments)}}
}
