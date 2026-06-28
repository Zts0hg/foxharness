package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/memory"
	providerpkg "github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type blockingLLMProvider struct {
	entered chan struct{}
	release chan struct{}
}

func testResolvedLLM(protocol string, model string) llmconfig.ResolvedConfig {
	return llmconfig.ResolvedConfig{
		ProviderID: "test-provider",
		Protocol:   protocol,
		BaseURL:    "http://127.0.0.1:1",
		Model:      model,
		Auth:       llmconfig.AuthNone,
	}
}

func TestNewAgentRunnerMissingLLMConfigReturnsHelpfulError(t *testing.T) {
	_, err := NewAgentRunner(context.Background(), AgentRunnerConfig{
		WorkDir:  t.TempDir(),
		Model:    "test-model",
		MaxTurns: 1,
	})
	if err == nil {
		t.Fatal("NewAgentRunner returned nil error, want missing config error")
	}
	if !strings.Contains(err.Error(), "missing LLM configuration") {
		t.Fatalf("error = %q, want missing LLM configuration", err.Error())
	}
}

func TestAgentRunnerSetModelUpdatesModel(t *testing.T) {
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	runner := &AgentRunner{
		workDir:          workDir,
		model:            "old-model",
		providerProtocol: "claude",
		llmConfig:        testResolvedLLM(llmconfig.ProtocolClaude, "old-model"),
		maxTurns:         3,
		store:            store,
		manager:          manager,
		llmProvider:      &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})},
		currentSession:   sess,
	}

	if err := runner.SetModel("new-model"); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if got := runner.Model(); got != "new-model" {
		t.Fatalf("Model() = %q, want new-model", got)
	}
	if runner.llmConfig.Model != "new-model" {
		t.Fatalf("llmConfig.Model = %q, want new-model", runner.llmConfig.Model)
	}
	if runner.llmConfig.Protocol != llmconfig.ProtocolClaude {
		t.Fatalf("llmConfig.Protocol = %q, want preserved claude", runner.llmConfig.Protocol)
	}
}

func TestAgentRunnerSetModelNilCallback(t *testing.T) {
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	runner := &AgentRunner{
		workDir:          workDir,
		model:            "old-model",
		providerProtocol: "openai",
		llmConfig:        testResolvedLLM(llmconfig.ProtocolOpenAI, "old-model"),
		maxTurns:         3,
		store:            store,
		manager:          manager,
		llmProvider:      &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})},
		currentSession:   sess,
		onModelChange:    nil,
	}

	if err := runner.SetModel("new-model"); err != nil {
		t.Fatalf("SetModel() with nil callback error = %v", err)
	}
	if got := runner.Model(); got != "new-model" {
		t.Fatalf("Model() = %q, want new-model", got)
	}
}

func TestAgentRunnerSetModelCallbackError(t *testing.T) {
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var callbackCalled bool
	runner := &AgentRunner{
		workDir:          workDir,
		model:            "old-model",
		providerProtocol: "openai",
		llmConfig:        testResolvedLLM(llmconfig.ProtocolOpenAI, "old-model"),
		maxTurns:         3,
		store:            store,
		manager:          manager,
		llmProvider:      &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})},
		currentSession:   sess,
		onModelChange: func(model string) error {
			callbackCalled = true
			return fmt.Errorf("write failed")
		},
	}

	if err := runner.SetModel("new-model"); err != nil {
		t.Fatalf("SetModel() should not fail when callback errors, got: %v", err)
	}
	if got := runner.Model(); got != "new-model" {
		t.Fatalf("Model() = %q, want new-model (switch should succeed)", got)
	}
	if !callbackCalled {
		t.Fatal("callback was not called")
	}
}

func TestAgentRunnerSetModelCallbackSuccess(t *testing.T) {
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	var receivedModel string
	runner := &AgentRunner{
		workDir:          workDir,
		model:            "old-model",
		providerProtocol: "openai",
		llmConfig:        testResolvedLLM(llmconfig.ProtocolOpenAI, "old-model"),
		maxTurns:         3,
		store:            store,
		manager:          manager,
		llmProvider:      &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})},
		currentSession:   sess,
		onModelChange: func(model string) error {
			receivedModel = model
			return nil
		},
	}

	if err := runner.SetModel("new-model"); err != nil {
		t.Fatalf("SetModel() error = %v", err)
	}
	if receivedModel != "new-model" {
		t.Fatalf("callback received %q, want new-model", receivedModel)
	}
}

func (p *blockingLLMProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*providerpkg.GenerateResponse, error) {
	select {
	case <-p.entered:
	default:
		close(p.entered)
	}

	select {
	case <-p.release:
		return &providerpkg.GenerateResponse{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestAgentRunnerSetPlanModeWhileRunIsActive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	provider := &blockingLLMProvider{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
	runner := &AgentRunner{
		workDir:        workDir,
		model:          "fake-model",
		enableThinking: false,
		enablePlanMode: false,
		maxTurns:       3,
		store:          store,
		manager:        manager,
		llmProvider:    provider,
		currentSession: sess,
	}

	runDone := make(chan error, 1)
	go func() {
		_, err := runner.Run(ctx, "hello", nil)
		runDone <- err
	}()

	select {
	case <-provider.entered:
	case <-time.After(time.Second):
		t.Fatal("Run did not reach the model call")
	}

	toggleDone := make(chan struct{})
	go func() {
		runner.SetPlanMode(true)
		close(toggleDone)
	}()

	select {
	case <-toggleDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("SetPlanMode blocked while Run was active")
	}
	if !runner.PlanMode() {
		t.Fatal("PlanMode() = false, want true")
	}

	close(provider.release)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not finish after provider release")
	}
}

func TestAgentRunnerContextUsage(t *testing.T) {
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	runner := &AgentRunner{
		workDir:        workDir,
		model:          "fake-model",
		enableThinking: false,
		enablePlanMode: false,
		maxTurns:       3,
		store:          store,
		manager:        manager,
		llmProvider:    &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})},
		currentSession: sess,
	}

	if got := runner.ContextUsage(); got != "0%" {
		t.Fatalf("empty ContextUsage() = %q, want 0%%", got)
	}
	if _, err := session.NewMessageLog(sess).Append("run-1", schema.Message{Role: schema.RoleUser, Content: "hello"}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if got := runner.ContextUsage(); got != "<1%" {
		t.Fatalf("ContextUsage() = %q, want <1%%", got)
	}
}

func TestAgentRunnerMessageHistory(t *testing.T) {
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := session.NewMessageLog(sess).Append("run-1", schema.Message{Role: schema.RoleUser, Content: "restore me"}); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	runner := &AgentRunner{
		workDir:        workDir,
		model:          "fake-model",
		enableThinking: false,
		enablePlanMode: false,
		maxTurns:       3,
		store:          store,
		manager:        manager,
		llmProvider:    &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})},
		currentSession: sess,
	}

	records, err := runner.MessageHistory()
	if err != nil {
		t.Fatalf("MessageHistory() error = %v", err)
	}
	if len(records) != 1 || records[0].Message.Content != "restore me" {
		t.Fatalf("MessageHistory() = %#v, want one restored user message", records)
	}
}

func TestAgentRunnerRunWithDisplayPersistsHumanPrompt(t *testing.T) {
	ctx := context.Background()
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	provider := &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})}
	runner := &AgentRunner{
		workDir:        workDir,
		model:          "fake-model",
		enableThinking: false,
		enablePlanMode: false,
		maxTurns:       3,
		store:          store,
		manager:        manager,
		llmProvider:    provider,
		currentSession: sess,
	}

	runDone := make(chan error, 1)
	go func() {
		_, err := runner.RunWithDisplay(ctx, "Review: pr-9", "/review pr-9", nil)
		runDone <- err
	}()
	select {
	case <-provider.entered:
	case <-time.After(time.Second):
		t.Fatal("RunWithDisplay did not reach model call")
	}
	close(provider.release)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("RunWithDisplay() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunWithDisplay did not finish")
	}

	records, err := runner.MessageHistory()
	if err != nil {
		t.Fatalf("MessageHistory() error = %v", err)
	}
	if len(records) < 1 {
		t.Fatalf("MessageHistory() = %#v, want user record", records)
	}
	if got := records[0].Message.Content; got != "Review: pr-9" {
		t.Fatalf("model content = %q, want expanded prompt", got)
	}
	if got := records[0].DisplayContent; got != "/review pr-9" {
		t.Fatalf("display content = %q, want original command", got)
	}
	if messages, err := session.NewMessageLog(sess).LoadMessages(); err != nil {
		t.Fatalf("LoadMessages() error = %v", err)
	} else if len(messages) < 1 || messages[0].Content != "Review: pr-9" {
		t.Fatalf("LoadMessages() = %#v, want model-visible expanded prompt", messages)
	}
	transcript, err := os.ReadFile(sess.TranscriptPath())
	if err != nil {
		t.Fatalf("ReadFile(transcript) error = %v", err)
	}
	if !strings.Contains(string(transcript), `"prompt":"/review pr-9"`) ||
		!strings.Contains(string(transcript), `"model_prompt":"Review: pr-9"`) {
		t.Fatalf("transcript missing display/model prompt split: %s", transcript)
	}
}

func TestAgentRunnerProjectInputHistoryFiltersAndOrdersProjectPrompts(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())

	current, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create(current) error = %v", err)
	}
	other, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create(other) error = %v", err)
	}
	feishu, err := manager.Create(session.CreateOptions{Source: session.SOURCEFeishu, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create(feishu) error = %v", err)
	}
	subagent, err := manager.Create(session.CreateOptions{Source: session.SOURCESubagent, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create(subagent) error = %v", err)
	}

	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	writeProjectHistoryRecords(t, current, []session.MessageRecord{
		projectHistoryRecord(0, base.Add(10*time.Minute), schema.Message{Role: schema.RoleUser, Content: "current older"}),
		projectHistoryRecord(1, base.Add(11*time.Minute), schema.Message{Role: schema.RoleUser, Content: "   "}),
		projectHistoryRecord(2, base.Add(12*time.Minute), schema.Message{Role: schema.RoleUser, ToolCallID: "call-1", Content: "tool result"}),
		projectHistoryRecord(3, base.Add(13*time.Minute), schema.Message{Role: schema.RoleUser, Content: "## Compacted Context Summary\n\nsummary"}),
		projectHistoryRecord(4, base.Add(14*time.Minute), schema.Message{Role: schema.RoleUser, Content: "current newest"}),
	})
	writeProjectHistoryRecords(t, other, []session.MessageRecord{
		projectHistoryRecord(0, base.Add(1*time.Minute), schema.Message{Role: schema.RoleUser, Content: "other older"}),
		projectHistoryRecord(1, base.Add(2*time.Minute), schema.Message{Role: schema.RoleAssistant, Content: "assistant ignored"}),
		{
			Seq:            2,
			Time:           base.Add(3 * time.Minute),
			DisplayContent: "/review pr-9",
			Message:        schema.Message{Role: schema.RoleUser, Content: "Review: pr-9"},
		},
	})
	writeProjectHistoryRecords(t, feishu, []session.MessageRecord{
		projectHistoryRecord(0, base.Add(20*time.Minute), schema.Message{Role: schema.RoleUser, Content: "feishu ignored"}),
	})
	writeProjectHistoryRecords(t, subagent, []session.MessageRecord{
		projectHistoryRecord(0, base.Add(21*time.Minute), schema.Message{Role: schema.RoleUser, Content: "subagent ignored"}),
	})

	runner := &AgentRunner{
		manager:        manager,
		currentSession: current,
	}
	history, err := runner.ProjectInputHistory(100)
	if err != nil {
		t.Fatalf("ProjectInputHistory() error = %v", err)
	}
	want := []string{"other older", "/review pr-9", "current older", "current newest"}
	if strings.Join(history, "\n") != strings.Join(want, "\n") {
		t.Fatalf("ProjectInputHistory() = %#v, want %#v", history, want)
	}
}

func TestAgentRunnerProjectInputHistoryCapsAtLimit(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	current, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create(current) error = %v", err)
	}
	other, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create(other) error = %v", err)
	}

	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	var records []session.MessageRecord
	for i := 0; i < 105; i++ {
		records = append(records, projectHistoryRecord(int64(i), base.Add(time.Duration(i)*time.Minute), schema.Message{
			Role:    schema.RoleUser,
			Content: fmt.Sprintf("prompt-%03d", i),
		}))
	}
	writeProjectHistoryRecords(t, other, records)
	writeProjectHistoryRecords(t, current, []session.MessageRecord{
		projectHistoryRecord(200, base.Add(200*time.Minute), schema.Message{Role: schema.RoleUser, Content: "current latest"}),
	})

	runner := &AgentRunner{
		manager:        manager,
		currentSession: current,
	}
	history, err := runner.ProjectInputHistory(100)
	if err != nil {
		t.Fatalf("ProjectInputHistory() error = %v", err)
	}
	if len(history) != 100 {
		t.Fatalf("len(ProjectInputHistory()) = %d, want 100", len(history))
	}
	if history[len(history)-1] != "current latest" {
		t.Fatalf("last history = %q, want current latest for first Up recall", history[len(history)-1])
	}
	for _, got := range history {
		if got == "prompt-000" || got == "prompt-001" || got == "prompt-002" || got == "prompt-003" || got == "prompt-004" || got == "prompt-005" {
			t.Fatalf("history includes capped old prompt %q: %#v", got, history[:8])
		}
	}
}

func TestAgentRunnerRestoresSessionStateBeforeMessage(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	store := memory.NewSessionStore(workDir, sessionDir)
	if err := os.WriteFile(filepath.Join(sessionDir, "PLAN.md"), []byte("old plan"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "TODO.md"), []byte("old todo"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := memory.NewStateHistory(store).SnapshotBeforeMessage(4); err != nil {
		t.Fatalf("SnapshotBeforeMessage() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "PLAN.md"), []byte("new plan"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sessionDir, "TODO.md"), []byte("new todo"), 0644); err != nil {
		t.Fatal(err)
	}

	runner := &AgentRunner{store: store}
	restored, err := runner.RestoreSessionStateBeforeMessage(4)
	if err != nil {
		t.Fatalf("RestoreSessionStateBeforeMessage() error = %v", err)
	}
	if !restored {
		t.Fatalf("RestoreSessionStateBeforeMessage() restored = false, want true")
	}
	assertFileContent(t, filepath.Join(sessionDir, "PLAN.md"), "old plan")
	assertFileContent(t, filepath.Join(sessionDir, "TODO.md"), "old todo")
}

func TestAgentRunnerRegistryIncludesTodoTools(t *testing.T) {
	runner := &AgentRunner{workDir: t.TempDir()}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(sess, &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})}, nil, func() string { return "" })

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

func writeProjectHistoryRecords(t *testing.T, sess *session.Session, records []session.MessageRecord) {
	t.Helper()
	f, err := os.OpenFile(sess.MessagesPath(), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("OpenFile(messages) error = %v", err)
	}
	defer f.Close()
	for _, record := range records {
		line, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("Marshal(record) error = %v", err)
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			t.Fatalf("Write(record) error = %v", err)
		}
	}
}

func projectHistoryRecord(seq int64, when time.Time, msg schema.Message) session.MessageRecord {
	return session.MessageRecord{
		Seq:     seq,
		RunID:   fmt.Sprintf("run-%d", seq),
		Time:    when,
		Kind:    session.MessageKindNormal,
		Message: msg,
	}
}

func TestFormatContextUsage(t *testing.T) {
	cases := []struct {
		used int
		max  int
		want string
	}{
		{used: 0, max: 128000, want: "0%"},
		{used: 1, max: 128000, want: "<1%"},
		{used: 1280, max: 128000, want: "1%"},
		{used: 1281, max: 128000, want: "2%"},
		{used: 128000, max: 128000, want: "100%"},
		{used: 1, max: 0, want: "unknown"},
	}
	for _, tc := range cases {
		if got := formatContextUsage(tc.used, tc.max); got != tc.want {
			t.Fatalf("formatContextUsage(%d, %d) = %q, want %q", tc.used, tc.max, got, tc.want)
		}
	}
}

func assertFileContent(t *testing.T, path string, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if got := string(data); got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

// TestInlineMemoryWriteSetsTrackerAndIndex verifies the inline write path
// (US2): a write_file whose relative path resolves into a memory directory is
// detected by the tracker (recorded on success via the engine's OnToolCalled),
// the memory is persisted and indexed, and an explicit forget via empty
// write_file content removes it from the regenerated index. (T012)
func TestInlineMemoryWriteSetsTrackerAndIndex(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	store := automemory.NewStore(home, workDir)
	hooks := automemory.NewPerRunHooks(nil, store, workDir)
	tracker := hooks.NewTracker()
	onToolCalled := hooks.RecordCallback(tracker)

	registry := tools.NewRegistry()
	registry.Register(tools.NewWriteFileTool(workDir))

	rel, err := filepath.Rel(workDir, filepath.Join(store.UserGlobalDir(), "user-role.md"))
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}
	content := "---\nname: user-role\ndescription: Staff engineer, terse answers.\ntype: user\n---\n\nThe user is a staff engineer.\n"
	args, _ := json.Marshal(map[string]string{"path": rel, "content": content})

	call := schema.ToolCall{ID: "c1", Name: "write_file", Arguments: args}
	res := registry.Execute(context.Background(), call)
	if res.IsError {
		t.Fatalf("write_file failed: %s", res.Output)
	}
	// The engine reports tool results post-execution via OnToolCalled; a failed
	// write must not set the flag (P2-2).
	onToolCalled(call, schema.ToolResult{IsError: true, Output: "boom"})
	if tracker.WroteMemory() {
		t.Fatalf("a failed write must not set the tracker flag")
	}
	onToolCalled(call, res)
	if !tracker.WroteMemory() {
		t.Fatalf("tracker must flag a successful inline memory write")
	}

	mems, err := store.Load(automemory.ScopeUserGlobal)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(mems) != 1 || mems[0].Name != "user-role" {
		t.Fatalf("inline memory not persisted: %+v", mems)
	}
	idx, _ := store.BuildIndex(automemory.ScopeUserGlobal)
	if !strings.Contains(idx, "user-role.md") {
		t.Fatalf("index missing inline memory entry:\n%s", idx)
	}

	forgetTracker := hooks.NewTracker()
	forgetArgs, _ := json.Marshal(map[string]string{"path": rel, "content": ""})
	forgetCall := schema.ToolCall{ID: "c2", Name: "write_file", Arguments: forgetArgs}
	forgetRes := registry.Execute(context.Background(), forgetCall)
	if forgetRes.IsError {
		t.Fatalf("forget write_file failed: %s", forgetRes.Output)
	}
	hooks.RecordCallback(forgetTracker)(forgetCall, forgetRes)
	if !forgetTracker.WroteMemory() {
		t.Fatalf("tracker must flag a successful empty-content forget")
	}
	idx2, _ := store.BuildIndex(automemory.ScopeUserGlobal)
	if strings.Contains(idx2, "user-role.md") {
		t.Fatalf("index still lists forgotten memory:\n%s", idx2)
	}
	if mems, _ := store.Load(automemory.ScopeUserGlobal); len(mems) != 0 {
		t.Fatalf("forgotten memory must no longer load: %+v", mems)
	}
}

// immediateMemoryRunner builds a runner whose provider returns a final message
// on the first call, so a Run completes in one turn. Used to exercise the
// post-run extraction wiring (T015).
func immediateMemoryRunner(t *testing.T) (*AgentRunner, *blockingLLMProvider) {
	t.Helper()
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	prov := &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})}
	close(prov.release) // return a final message immediately, no tool calls

	runner := &AgentRunner{
		workDir:        workDir,
		model:          "fake-model",
		maxTurns:       3,
		store:          store,
		autoMemory:     automemory.NewStore(home, workDir),
		manager:        manager,
		llmProvider:    prov,
		currentSession: sess,
	}
	return runner, prov
}

func TestRunFiresExtractionHookWithTracker(t *testing.T) {
	runner, _ := immediateMemoryRunner(t)

	called := 0
	var gotTracker *automemory.Tracker
	runner.extractionFire = func(s *session.Session, runID string, tr *automemory.Tracker) {
		called++
		gotTracker = tr
	}

	result, err := runner.Run(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result == nil || result.FinalMessage == "" {
		t.Fatalf("unexpected run result: %+v", result)
	}
	if called != 1 {
		t.Fatalf("extraction hook fired %d times, want exactly 1 at run end", called)
	}
	if gotTracker == nil {
		t.Fatalf("extraction hook must receive the run tracker for mutual exclusion")
	}
}

func TestPanickingExtractionHookDoesNotAffectRunResult(t *testing.T) {
	runner, _ := immediateMemoryRunner(t)
	runner.extractionFire = func(s *session.Session, runID string, tr *automemory.Tracker) {
		panic("extraction blew up")
	}

	result, err := runner.Run(context.Background(), "hello", nil)
	if err != nil {
		t.Fatalf("a panicking extraction hook must not fail the run, got %v", err)
	}
	if result == nil || result.FinalMessage == "" {
		t.Fatalf("a panicking extraction hook must not corrupt the result: %+v", result)
	}
}

// immediateCountingProvider returns a final message on every Generate and counts
// calls, so a run ends in one turn and extraction's Generate is observable.
type immediateCountingProvider struct {
	mu sync.Mutex
	n  int
}

func (p *immediateCountingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*providerpkg.GenerateResponse, error) {
	p.mu.Lock()
	p.n++
	p.mu.Unlock()
	return &providerpkg.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
	}, nil
}

func (p *immediateCountingProvider) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.n
}

// TestOneShotRunAwaitsExtraction proves the one-shot path drains the extraction
// goroutine before returning: after Run + WaitForExtraction, the extraction
// Generate call has completed (P2-A). Without awaiting, the process would exit
// first.
func TestOneShotRunAwaitsExtraction(t *testing.T) {
	workDir := t.TempDir()
	home := t.TempDir()
	manager := session.NewManagerWithHome(workDir, home)
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	prov := &immediateCountingProvider{}
	runner := &AgentRunner{
		workDir:        workDir,
		model:          "fake-model",
		maxTurns:       3,
		store:          store,
		autoMemory:     automemory.NewStore(home, workDir),
		manager:        manager,
		llmProvider:    prov,
		currentSession: sess,
	}

	if _, err := runner.Run(context.Background(), "hello", nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	runner.WaitForExtraction()

	// 1 Generate for the main run, plus >=1 for the extraction pass.
	if got, want := prov.count(), 2; got < want {
		t.Fatalf("extraction not awaited: Generate calls = %d, want >= %d", got, want)
	}
}
