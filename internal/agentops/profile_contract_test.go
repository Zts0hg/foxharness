package agentops

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestPFAOP001CurrentProfileSnapshotIsFlatAndRestrictionsOnlyNarrow(t *testing.T) {
	runner := newAgentOpsProfileRunner(t, &agentOpsProfileProvider{final: "done"}, &agentOpsProfileMessenger{})
	sess := &session.Session{ID: "session", RootDir: t.TempDir()}
	registry := runner.buildRegistry(Task{ChatID: "chat", Text: "incident"}, sess)

	got := currentAgentOpsProfileSnapshot{
		name: "AgentOpsTask", sessionSource: string(session.SOURCEFeishu), workspace: runner.workDir,
		providerScope: "task-fixed", logDir: runner.logDir, maxTurns: 24,
		taskTimeout: runner.taskTimeoutOrDefault(), maxConcurrent: runner.concurrentTaskLimit(),
		permission: "ask", memory: true, automaticCompaction: true, extraction: "fire-and-forget",
		observation: "terminal-only", canonicalTools: agentOpsProfileToolNames(registry.GetAvailableTools()),
	}
	want := currentAgentOpsProfileSnapshot{
		name: "AgentOpsTask", sessionSource: string(session.SOURCEFeishu), workspace: runner.workDir,
		providerScope: "task-fixed", logDir: runner.logDir, maxTurns: 24,
		taskTimeout: 5 * time.Minute, maxConcurrent: 4,
		permission: "ask", memory: true, automaticCompaction: true, extraction: "fire-and-forget",
		observation:    "terminal-only",
		canonicalTools: "bash,delegate_task,edit_file,log_search,read_file,read_todo,update_todo,write_file",
	}
	if got != want {
		t.Fatalf("current AgentOps profile = %#v, want %#v", got, want)
	}

	filtered := tools.NewFilteredRegistry(registry, []string{"log_search", "not-in-profile"})
	if names := agentOpsProfileToolNames(filtered.GetAvailableTools()); names != "log_search" {
		t.Fatalf("restricted tools = %q, want log_search", names)
	}
	if result := filtered.Execute(context.Background(), schema.ToolCall{ID: "expand", Name: "not-in-profile"}); !result.IsError {
		t.Fatalf("restriction expanded profile: %#v", result)
	}

	source := readAgentOpsPackageSource(t, "runner.go")
	for _, required := range []string{"MaxTurns:     24", "snapshotTaskProvider(r.provider)"} {
		if !strings.Contains(source, required) {
			t.Fatalf("runner source missing frozen profile setting %q", required)
		}
	}
}

func TestPFAOP002TaskIdentityAndIncidentPromptAreExact(t *testing.T) {
	task := Task{
		TaskID: "task-1", ChatID: "chat-1", SenderID: "sender-1", MessageID: "message-1",
		Text: "  inspect payment latency  ", Service: "payment", Since: "15m", Query: "timeout",
	}
	copy := task
	copy.Text = "changed"
	if task.Text != "  inspect payment latency  " || task.TaskID != "task-1" || task.ChatID != "chat-1" || task.SenderID != "sender-1" || task.MessageID != "message-1" {
		t.Fatalf("typed task identity mutated: %#v", task)
	}

	want := `
你正在作为 AgentOps 小助手处理一条来自团队 IM 的故障分析任务。

用户原始请求：
` + "  inspect payment latency  " + `

工作规则：
1. 先收集证据，再给结论。
2. 优先使用 log_search、read_file、bash 中的只读命令进行分析。
3. 不要在没有证据时猜测根因。
4. 如果需要修改代码，必须做最小修改，并运行相关测试。
5. 如果需要执行重启、删除、发布、kubectl、terraform、git push 等高危动作，必须等待审批 Middleware 放行。
6. 最终回复必须包含：现象、证据、根因判断、修改内容、验证结果、仍需人工确认的风险。

如果日志不足，请明确说明还缺少哪些信息。
`
	if got := BuildPrompt(task); got != want {
		t.Fatalf("incident prompt = %q, want %q", got, want)
	}
	parsed := Parse("/new inspect payment")
	if parsed.Text != "/new inspect payment" || parsed.Query != parsed.Text {
		t.Fatalf("AgentOps interpreted a presentation directive: %#v", parsed)
	}
}

func TestPFAOP003EveryTaskCreatesFreshIsolatedSession(t *testing.T) {
	model := &agentOpsProfileProvider{final: "analysis complete"}
	messenger := &agentOpsProfileMessenger{}
	runner := newAgentOpsProfileRunner(t, model, messenger)

	for _, task := range []Task{
		{TaskID: "task-1", ChatID: "chat", SenderID: "sender", MessageID: "message-1", Text: "/new investigate first"},
		{TaskID: "task-2", ChatID: "chat", SenderID: "sender", MessageID: "message-2", Text: "new session investigate second"},
	} {
		if err := runner.run(context.Background(), task); err != nil {
			t.Fatalf("run(%s) error = %v", task.TaskID, err)
		}
	}

	sessions, err := runner.sessions.List(session.LookupOptions{Source: session.SOURCEFeishu, UserID: "sender", ChatID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].ID == sessions[1].ID || sessions[0].RootDir == sessions[1].RootDir {
		t.Fatalf("fresh task sessions = %#v", sessions)
	}
	seenPrompts := map[string]bool{}
	seenRuns := map[string]bool{}
	for _, sess := range sessions {
		if sess.Source != session.SOURCEFeishu || sess.WorkDir != runner.workDir || sess.UserID != "sender" || sess.ChatID != "chat" {
			t.Fatalf("session metadata = %#v", sess)
		}
		records, err := session.NewMessageLog(sess).LoadRecords()
		if err != nil || len(records) != 2 {
			t.Fatalf("session %s records = %#v, %v", sess.ID, records, err)
		}
		if records[0].RunID == "" || seenRuns[records[0].RunID] || records[1].RunID != records[0].RunID {
			t.Fatalf("session %s run identity = %#v", sess.ID, records)
		}
		seenRuns[records[0].RunID] = true
		seenPrompts[records[0].Message.Content] = true
	}
	for _, text := range []string{"/new investigate first", "new session investigate second"} {
		if !seenPrompts[BuildPrompt(Task{Text: text})] {
			t.Fatalf("fresh session lost literal task %q: %#v", text, seenPrompts)
		}
	}
}

func TestPFAOP005SessionNoticePrecedesRunAndArtifacts(t *testing.T) {
	model := &agentOpsProfileProvider{final: "resolved"}
	messenger := &agentOpsProfileMessenger{}
	runner := newAgentOpsProfileRunner(t, model, messenger)
	if err := runner.run(context.Background(), Task{TaskID: "task", ChatID: "chat", SenderID: "sender", Text: "incident"}); err != nil {
		t.Fatal(err)
	}
	texts := messenger.snapshot()
	if len(texts) != 2 || !strings.HasPrefix(texts[0], "已创建 AgentOps Session: ") || !strings.HasSuffix(texts[0], "\n开始分析。") || !strings.HasPrefix(texts[1], "resolved\n\nSession: ") {
		t.Fatalf("message ordering/content = %#v", texts)
	}
	source := readAgentOpsPackageSource(t, "runner.go")
	notice := strings.Index(source, "DeliveryStageSession")
	ensure := strings.Index(source, "store.EnsureFiles()")
	if notice < 0 || ensure < 0 || notice >= ensure {
		t.Fatalf("session notice/EnsureFiles source order = %d/%d", notice, ensure)
	}
}

func TestPFAOP006And009ExactCapabilityAndAbsentInteractionSurface(t *testing.T) {
	runner := newAgentOpsProfileRunner(t, &agentOpsProfileProvider{final: "done"}, &agentOpsProfileMessenger{})
	sess := &session.Session{ID: "session", RootDir: t.TempDir()}
	registry := runner.buildRegistry(Task{ChatID: "chat", Text: "incident"}, sess)
	if got, want := agentOpsProfileToolNames(registry.GetAvailableTools()), "bash,delegate_task,edit_file,log_search,read_file,read_todo,update_todo,write_file"; got != want {
		t.Fatalf("tool surface = %q, want %q", got, want)
	}
	for _, forbidden := range []string{"skill", "ask_user_question", "AskUserQuestion", "submit_plan", "compact", "rewind"} {
		if agentOpsRegistryHasTool(registry, forbidden) {
			t.Fatalf("AgentOps profile exposed %q", forbidden)
		}
	}
	permissionRegistry, ok := registry.(tools.PermissionRegistry)
	if !ok {
		t.Fatalf("registry %T lacks permission assessment", registry)
	}
	assessment, found, err := permissionRegistry.AssessPermission(
		"log_search",
		toolpolicy.Context{Workspace: runner.workDir, CWD: runner.workDir},
		[]byte(`{"service":"payment","query":"error"}`),
	)
	if err != nil || !found || assessment.Behavior != toolpolicy.BehaviorFastAllow || !assessment.ReadOnly {
		t.Fatalf("log_search permission = %#v/%v/%v", assessment, found, err)
	}
	for _, definition := range registry.GetAvailableTools() {
		if registry.IsParallelSafe(definition.Name) {
			t.Fatalf("permission-decorated AgentOps tool %q unexpectedly parallel-safe", definition.Name)
		}
	}
}

func TestPFAOP007LogSearchSchemaAndExecutionMatrix(t *testing.T) {
	logDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(logDir, "payment.log"), []byte("INFO ready\nError first\nERROR second\nerror third\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := NewLogSearchTool(logDir)
	definition := tool.Definition()
	if definition.Name != "log_search" {
		t.Fatalf("definition name = %q", definition.Name)
	}
	inputSchema, ok := definition.InputSchema.(map[string]interface{})
	if !ok {
		t.Fatalf("input schema = %#v, want object schema", definition.InputSchema)
	}
	required, _ := inputSchema["required"].([]string)
	if strings.Join(required, ",") != "service,query" {
		t.Fatalf("required schema = %#v", required)
	}
	properties, _ := inputSchema["properties"].(map[string]interface{})
	if _, ok := properties["limit"]; !ok {
		t.Fatalf("optional limit missing from schema: %#v", properties)
	}

	for _, tc := range []struct {
		name string
		args string
		want string
	}{
		{name: "case insensitive and ordered", args: `{"service":"payment","query":"ERROR","limit":2}`, want: "Error first\nERROR second"},
		{name: "out of range falls back to fifty", args: `{"service":"payment","query":"error","limit":201}`, want: "Error first\nERROR second\nerror third"},
		{name: "no match", args: `{"service":"payment","query":"missing"}`, want: "没有匹配日志。"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tool.Execute(context.Background(), []byte(tc.args))
			if err != nil || got != tc.want {
				t.Fatalf("Execute() = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
	for _, args := range []string{`{`, `{}`, `{"service":"payment"}`, `{"query":"error"}`} {
		if _, err := tool.Execute(context.Background(), []byte(args)); err == nil {
			t.Fatalf("malformed/missing input %q accepted", args)
		}
	}
	if _, err := NewLogSearchTool(filepath.Join(logDir, "missing")).Execute(context.Background(), []byte(`{"service":"payment","query":"error"}`)); err == nil || !strings.Contains(err.Error(), "读取日志失败") {
		t.Fatalf("open error = %v", err)
	}
}

func TestPFAOP011ApprovalEvidenceIsTaskAndSessionBounded(t *testing.T) {
	sess := &session.Session{ID: "session", RootDir: t.TempDir()}
	if _, err := session.NewMessageLog(sess).Append("prior", schema.Message{Role: schema.RoleUser, Content: "prior incident"}); err != nil {
		t.Fatal(err)
	}
	current := BuildPrompt(Task{TaskID: "task", ChatID: "chat", Text: "current incident"})
	provider := agentOpsPermissionEvidenceProvider(sess, current)
	request := permission.Request{
		ToolName: "write_file", Arguments: `{"path":"fix.go","content":"x"}`, Action: "write fix.go",
		Risk: permission.RiskHigh, ToolCall: schema.ToolCall{ID: "call", Name: "write_file", Arguments: []byte(`{"path":"fix.go","content":"x"}`)},
	}
	evidence := provider(request)
	if strings.Count(evidence.Text, "prior incident") != 1 || strings.Count(evidence.Text, current) != 1 {
		t.Fatalf("approval evidence duplicated or crossed task boundary:\n%s", evidence.Text)
	}
	for _, required := range []string{`"tool":"write_file"`, `"action":"write fix.go"`} {
		if !strings.Contains(evidence.Trusted, required) {
			t.Fatalf("trusted evidence missing %q: %s", required, evidence.Trusted)
		}
	}
	if request.Risk != permission.RiskHigh || request.Arguments != `{"path":"fix.go","content":"x"}` || request.ToolCall.ID != "call" {
		t.Fatalf("approval request lost risk, arguments, or tool-call correlation: %#v", request)
	}

	if _, err := session.NewMessageLog(sess).Append("current", schema.Message{Role: schema.RoleUser, Content: current}); err != nil {
		t.Fatal(err)
	}
	if got := agentOpsPermissionEvidenceProvider(sess, current)(request); strings.Count(got.Text, current) != 1 {
		t.Fatalf("current incident prompt duplicated:\n%s", got.Text)
	}
}

func TestPFAOP013ContextCompositionContainsOnlyTaskRuntimeFragments(t *testing.T) {
	runner := newAgentOpsProfileRunner(t, &agentOpsProfileProvider{final: "done"}, &agentOpsProfileMessenger{})
	sess, err := runner.sessions.Create(session.CreateOptions{Source: session.SOURCEFeishu, WorkDir: runner.workDir, UserID: "sender", ChatID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runner.workDir, "AGENTS.md"), []byte("AGENTOPS_PROJECT_INSTRUCTION\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sess.MemoryPath(), []byte("AGENTOPS_SESSION_MEMORY\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := automemory.NewStore(runner.sessions.HomeDir(), runner.workDir)
	if err := store.Save(automemory.Memory{Name: "agentops-profile", Description: "incident memory", Type: automemory.TypeProject, Body: "Known incident fact.\n\n**Why:** test.\n**How to apply:** inject."}); err != nil {
		t.Fatal(err)
	}
	composed, err := runner.buildComposer(sess, store).Compose("incident")
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"AGENTOPS_PROJECT_INSTRUCTION", "AGENTOPS_SESSION_MEMORY", "agentops-profile.md"} {
		if strings.Count(composed, fragment) != 1 {
			t.Fatalf("prompt contains %q %d times:\n%s", fragment, strings.Count(composed, fragment), composed)
		}
	}
	for _, forbidden := range []string{"Available Skills", "Collaboration Mode", "ask_user_question", "Formal Plan"} {
		if strings.Contains(composed, forbidden) {
			t.Fatalf("AgentOps prompt contains UI/skill fragment %q:\n%s", forbidden, composed)
		}
	}
	if _, err := os.Stat(sess.CheckpointsLogPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("AgentOps composition created checkpoint state: %v", err)
	}
}

func TestPFAOP015ExtractionIsRunBoundedFireAndForget(t *testing.T) {
	workDir := t.TempDir()
	store := automemory.NewStore(t.TempDir(), workDir)
	model := &agentOpsExtractionBarrierProvider{started: make(chan struct{}), release: make(chan struct{})}
	hooks := automemory.NewPerRunHooks(model, store, workDir)
	sess := &session.Session{ID: "session", RootDir: t.TempDir()}
	if _, err := session.NewMessageLog(sess).Append("run-42", schema.Message{Role: schema.RoleUser, Content: "extract this incident"}); err != nil {
		t.Fatal(err)
	}
	(&Runner{}).fireMemoryExtraction(hooks, sess, "run-42", hooks.NewTracker())
	select {
	case <-model.started:
	case <-time.After(time.Second):
		t.Fatal("extraction did not start")
	}
	close(model.release)
}

func TestPFAOP016And017TerminalPresentationAndArtifactsAreCorrelated(t *testing.T) {
	model := &agentOpsProfileProvider{final: "incident resolved"}
	messenger := &agentOpsProfileMessenger{}
	runner := newAgentOpsProfileRunner(t, model, messenger)
	task := Task{TaskID: "task", ChatID: "chat", SenderID: "sender", MessageID: "message", Text: "incident"}
	if err := runner.run(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	texts := messenger.snapshot()
	if len(texts) != 2 {
		t.Fatalf("messages = %#v, want session notice and final only", texts)
	}
	sess, err := runner.sessions.Latest(session.LookupOptions{Source: session.SOURCEFeishu, UserID: "sender", ChatID: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	records, err := session.NewMessageLog(sess).LoadRecords()
	if err != nil || len(records) != 2 || records[0].RunID == "" || records[1].RunID != records[0].RunID {
		t.Fatalf("records = %#v, %v", records, err)
	}
	wantFinal := "incident resolved\n\nSession: " + sess.ID + "\nRun: " + records[0].RunID + "\nTrace: " + filepath.Join(sess.RunsDir(), records[0].RunID, "trace.jsonl") + "\nMetrics: " + filepath.Join(sess.RunsDir(), records[0].RunID, "metrics.jsonl")
	if texts[1] != wantFinal {
		t.Fatalf("final presentation = %q, want %q", texts[1], wantFinal)
	}
	for _, path := range []string{
		filepath.Join(sess.RunsDir(), records[0].RunID, "run.json"),
		filepath.Join(sess.RunsDir(), records[0].RunID, "trace.jsonl"),
		filepath.Join(sess.RunsDir(), records[0].RunID, "metrics.jsonl"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("run artifact %s: %v", path, err)
		}
	}
	for _, forbidden := range []string{"Thinking", "Tool:", "Transcript:"} {
		if strings.Contains(strings.Join(texts, "\n"), forbidden) {
			t.Fatalf("AgentOps exposed presentation-only label %q: %#v", forbidden, texts)
		}
	}
}

func TestPFAOP019CoreHasNoLarkHTTPOrTerminalDependency(t *testing.T) {
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
			for _, forbidden := range []string{"larksuite", "internal/agentops", "net/http", "AGENTOPS_", "AgentOps 任务"} {
				if strings.Contains(string(data), forbidden) {
					t.Fatalf("core file %s contains AgentOps transport token %q", filepath.Join(dir, entry.Name()), forbidden)
				}
			}
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source := readAgentOpsPackageSource(t, entry.Name())
		for _, forbidden := range []string{"larksuite", "net/http", "os.Stdout", "fmt.Print"} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("AgentOps runtime-control file %s contains transport/output token %q", entry.Name(), forbidden)
			}
		}
	}
}

type currentAgentOpsProfileSnapshot struct {
	name, sessionSource, workspace, providerScope, logDir, permission, extraction, observation, canonicalTools string
	maxTurns, maxConcurrent                                                                                    int
	taskTimeout                                                                                                time.Duration
	memory, automaticCompaction                                                                                bool
}

type agentOpsProfileProvider struct {
	mu       sync.Mutex
	final    string
	requests [][]schema.Message
}

func (p *agentOpsProfileProvider) Generate(_ context.Context, messages []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.mu.Lock()
	p.requests = append(p.requests, append([]schema.Message(nil), messages...))
	p.mu.Unlock()
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: p.final}}, nil
}

func (*agentOpsProfileProvider) ProviderProtocol() string { return "scripted" }
func (*agentOpsProfileProvider) ModelName() string        { return "claude-4-sonnet" }

type agentOpsExtractionBarrierProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *agentOpsExtractionBarrierProvider) Generate(ctx context.Context, _ []schema.Message, _ []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: `{}`}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type agentOpsProfileMessenger struct {
	mu    sync.Mutex
	texts []string
}

func (m *agentOpsProfileMessenger) SendText(_ context.Context, _ string, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.texts = append(m.texts, text)
	return nil
}

func (m *agentOpsProfileMessenger) snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.texts...)
}

func newAgentOpsProfileRunner(t *testing.T, model provider.LLMProvider, messenger Messenger) *Runner {
	t.Helper()
	workDir := t.TempDir()
	runner := NewRunner(model, workDir, t.TempDir(), messenger, approval.NewStore())
	runner.sessions = session.NewManagerWithHome(workDir, t.TempDir())
	return runner
}

func agentOpsProfileToolNames(definitions []schema.ToolDefinition) string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func agentOpsRegistryHasTool(registry tools.Registry, name string) bool {
	for _, definition := range registry.GetAvailableTools() {
		if definition.Name == name {
			return true
		}
	}
	return false
}

func readAgentOpsPackageSource(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
