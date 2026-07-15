package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type bigOutputTool struct {
	name   string
	output string
}

func (t *bigOutputTool) Name() string { return t.name }
func (t *bigOutputTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.name,
		Description: "test tool",
		InputSchema: map[string]any{"type": "object"},
	}
}
func (t *bigOutputTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	return t.output, nil
}

type sequencedProvider struct {
	responses []*provider.GenerateResponse
	call      int
	seen      [][]schema.Message
	seenTools [][]string
}

func (p *sequencedProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.seen = append(p.seen, append([]schema.Message(nil), messages...))
	names := make([]string, 0, len(availableTools))
	for _, definition := range availableTools {
		names = append(names, definition.Name)
	}
	p.seenTools = append(p.seenTools, names)
	idx := p.call
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	p.call++
	return p.responses[idx], nil
}

type optionCapturingProvider struct {
	generateCalls int
	optionCalls   int
	options       []provider.GenerateOptions
}

func (p *optionCapturingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	p.generateCalls++
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

func (p *optionCapturingProvider) GenerateWithOptions(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition, options provider.GenerateOptions) (*provider.GenerateResponse, error) {
	p.optionCalls++
	p.options = append(p.options, options)
	return &provider.GenerateResponse{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
}

type engineTurnRegistry struct {
	tools.Registry
	turns int
}

func (r *engineTurnRegistry) BeginTurn() {
	r.turns++
	r.Register(&bigOutputTool{name: "turn_tool", output: "ok"})
}

func TestEngineUsesGenerateOptionsForEffortOverride(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &optionCapturingProvider{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1, EffortOverride: "high"})
	if _, err := eng.RunWithReporter(context.Background(), sess, "test", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.generateCalls != 0 {
		t.Fatalf("Generate calls = %d, want 0 when effort override is set", p.generateCalls)
	}
	if p.optionCalls != 1 || len(p.options) != 1 || p.options[0].Effort != "high" {
		t.Fatalf("GenerateWithOptions calls/options = %d/%#v, want high", p.optionCalls, p.options)
	}
}

func TestEngineUsesDefaultGenerateWithoutEffortOverride(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &optionCapturingProvider{}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 1})
	if _, err := eng.RunWithReporter(context.Background(), sess, "test", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if p.generateCalls != 1 {
		t.Fatalf("Generate calls = %d, want 1 without effort override", p.generateCalls)
	}
	if p.optionCalls != 0 {
		t.Fatalf("GenerateWithOptions calls = %d, want 0 without effort override", p.optionCalls)
	}
}

func TestEngineBeginsRegistryTurnBeforeToolDiscovery(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := &engineTurnRegistry{Registry: tools.NewRegistry()}
	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call-turn",
				Name:      "turn_tool",
				Arguments: json.RawMessage(`{}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 3})

	if _, err := eng.RunWithReporter(context.Background(), sess, "test", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if registry.turns != 2 {
		t.Fatalf("BeginTurn calls = %d, want 2", registry.turns)
	}
	if len(p.seenTools) != 2 || !containsString(p.seenTools[0], "turn_tool") {
		t.Fatalf("provider tool surfaces = %#v, want turn_tool on first call", p.seenTools)
	}
}

func TestEngineCompletionGateInjectsReminderThenAllowsCompletion(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "premature"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	gateCalls := 0
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{
		MaxTurns: 3,
		CompletionGate: func() string {
			gateCalls++
			if gateCalls == 1 {
				return "submit_plan is still required"
			}
			return ""
		},
	})

	result, err := eng.RunWithReporter(context.Background(), sess, "test", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result.FinalMessage != "done" || p.call != 2 {
		t.Fatalf("result = %#v, provider calls = %d, want done after 2", result, p.call)
	}
	if len(p.seen) < 2 || !messagesContain(p.seen[1], "submit_plan is still required") {
		t.Fatalf("second call missing completion reminder: %#v", p.seen)
	}
}

func TestEngineCompletionGateFailsAfterRepeatedUnsatisfiedFinal(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "premature"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "still premature"}},
	}}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{
		MaxTurns:       3,
		CompletionGate: func() string { return "update_todo is still required" },
	})

	result, err := eng.RunWithReporter(context.Background(), sess, "test", nil)
	if err == nil || !strings.Contains(err.Error(), "completion gate remained unsatisfied") {
		t.Fatalf("RunWithReporter() error = %v, want unsatisfied completion gate", err)
	}
	if result == nil || p.call != 2 {
		t.Fatalf("result = %#v, provider calls = %d, want partial result after 2", result, p.call)
	}
}

func TestEngineCompletionGateGrantsRetryWhenReminderChanges(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "plan missing"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "checklist missing"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}
	reminders := []string{
		"submit_plan is still required",
		"update_todo is now required",
		"",
	}
	gateCalls := 0
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{
		MaxTurns: 4,
		CompletionGate: func() string {
			reminder := reminders[gateCalls]
			gateCalls++
			return reminder
		},
	})

	result, err := eng.RunWithReporter(context.Background(), sess, "test", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result == nil || result.FinalMessage != "done" || p.call != 3 {
		t.Fatalf("result = %#v, provider calls = %d, want done after both gate retries", result, p.call)
	}
	if len(p.seen) < 3 || !messagesContain(p.seen[1], reminders[0]) || !messagesContain(p.seen[2], reminders[1]) {
		t.Fatalf("provider history missing phase-specific reminders: %#v", p.seen)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func messagesContain(messages []schema.Message, want string) bool {
	for _, message := range messages {
		if strings.Contains(message.Content, want) {
			return true
		}
	}
	return false
}

type usageReportingProvider struct {
	usage   schema.Usage
	content string
}

func (p *usageReportingProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	content := p.content
	if content == "" {
		content = "done"
	}
	return &provider.GenerateResponse{
		Message: &schema.Message{Role: schema.RoleAssistant, Content: content},
		Usage:   p.usage,
	}, nil
}

func TestCallModelUsesGenerateResponse(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	p := &usageReportingProvider{
		usage: schema.Usage{InputTokens: 1234, OutputTokens: 56},
	}
	eng := NewAgentEngine(p, tools.NewRegistry(), workDir, staticComposer{}, Config{MaxTurns: 2})

	result, err := eng.RunWithReporter(context.Background(), sess, "hello", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result.FinalMessage != "done" {
		t.Fatalf("FinalMessage = %q, want done", result.FinalMessage)
	}

	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages() error = %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(messages))
	}
	assistant := messages[len(messages)-1]
	if assistant.Role != schema.RoleAssistant {
		t.Fatalf("last message role = %q, want assistant", assistant.Role)
	}
	if assistant.Usage == nil {
		t.Fatalf("assistant.Usage = nil, want populated usage")
	}
	if assistant.Usage.InputTokens != 1234 || assistant.Usage.OutputTokens != 56 {
		t.Fatalf("assistant.Usage = %#v, want {InputTokens:1234, OutputTokens:56}", assistant.Usage)
	}
}

func TestEngineRequiresTodoUpdateBeforeFinalResponse(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	todoPath := filepath.Join(sess.RootDir, "TODO.md")
	if err := os.WriteFile(todoPath, []byte("# TODO\n\n- [ ] Finish report\n"), 0644); err != nil {
		t.Fatalf("seed TODO.md: %v", err)
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "analysis complete"}},
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call_update_todo",
				Name:      "update_todo",
				Arguments: json.RawMessage(`{"content":"# TODO\n\n- [x] Finish report\n"}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}

	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 5})
	result, err := eng.RunWithReporter(context.Background(), sess, "finish report", nil)
	if err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}
	if result.FinalMessage != "done" {
		t.Fatalf("FinalMessage = %q, want done", result.FinalMessage)
	}
	if p.call != 3 {
		t.Fatalf("provider calls = %d, want 3", p.call)
	}
	data, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("read TODO.md: %v", err)
	}
	if strings.Contains(string(data), "- [ ] Finish report") || !strings.Contains(string(data), "- [x] Finish report") {
		t.Fatalf("TODO.md was not marked complete:\n%s", data)
	}

	var foundReminder bool
	if len(p.seen) < 2 {
		t.Fatalf("provider saw %d calls, want at least 2", len(p.seen))
	}
	for _, msg := range p.seen[1] {
		if strings.Contains(msg.Content, "TODO.md still has incomplete checklist items") {
			foundReminder = true
			break
		}
	}
	if !foundReminder {
		t.Fatalf("second model call missing TODO completion reminder: %#v", p.seen[1])
	}
}

func TestEngine_FullCompactionFlow(t *testing.T) {
	t.Setenv("FOXHARNESS_DISABLE_COMPACT", "")
	t.Setenv("FOXHARNESS_DISABLE_AUTO_COMPACT", "")

	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()

	responses := []*provider.GenerateResponse{
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "<summary>auto summary</summary>"}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}
	p := &sequencedProvider{responses: responses}

	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 5})

	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = "test-model"
	compCfg.RecentKeep = 1
	compCfg.SessionDir = sess.RootDir
	compCfg.TranscriptPath = sess.TranscriptPath()
	compCfg.Estimator = compaction.RoughEstimator{}
	compCfg.AutoCompactThreshold = 1
	compactor, err := compaction.NewCompactor(p, compCfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	eng.WithCompactor(compactor)

	log := session.NewMessageLog(sess)
	for i := 0; i < 6; i++ {
		_, err := log.Append("seed-run", schema.Message{
			Role:    schema.RoleAssistant,
			Content: strings.Repeat("legacy ", 200),
		})
		if err != nil {
			t.Fatalf("seed Append: %v", err)
		}
	}

	if _, err := eng.RunWithReporter(context.Background(), sess, "hello", nil); err != nil {
		t.Fatalf("RunWithReporter: %v", err)
	}

	if p.call < 1 {
		t.Fatalf("expected provider to be called at least once, got %d", p.call)
	}
}

// inMemoryFS records all WriteFile calls so a test can prove the engine
// routed tool-result persistence through the injected FileSystem instead
// of touching the real disk.
type inMemoryFS struct {
	writes map[string][]byte
}

func newInMemoryFS() *inMemoryFS { return &inMemoryFS{writes: map[string][]byte{}} }

func (f *inMemoryFS) WriteFile(path string, data []byte, _ os.FileMode) error {
	buf := make([]byte, len(data))
	copy(buf, data)
	f.writes[path] = buf
	return nil
}
func (f *inMemoryFS) Stat(path string) (os.FileInfo, error) {
	if _, ok := f.writes[path]; ok {
		return nil, nil
	}
	return nil, os.ErrNotExist
}
func (f *inMemoryFS) MkdirAll(_ string, _ os.FileMode) error { return nil }

func TestEngine_ToolResultsUseInjectedFileSystem(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()
	largeOutput := strings.Repeat("Z", 60000)
	registry.Register(&bigOutputTool{name: "big_dump", output: largeOutput})

	p := &sequencedProvider{responses: []*provider.GenerateResponse{
		{Message: &schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call_mem",
				Name:      "big_dump",
				Arguments: json.RawMessage(`{}`),
			}},
		}},
		{Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}}

	fs := newInMemoryFS()
	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 4})
	eng.WithFileSystem(fs)

	if _, err := eng.RunWithReporter(context.Background(), sess, "go", nil); err != nil {
		t.Fatalf("RunWithReporter: %v", err)
	}

	if len(fs.writes) == 0 {
		t.Fatalf("expected at least one write to the injected filesystem")
	}
	if _, err := os.Stat(filepath.Join(sess.ToolResultsDir(), "call_mem.txt")); err == nil {
		t.Fatalf("engine wrote to disk despite injected in-memory FileSystem")
	}
}

func TestEngine_ToolResultPersistence(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	registry := tools.NewRegistry()
	largeOutput := strings.Repeat("X", 60000)
	registry.Register(&bigOutputTool{name: "big_dump", output: largeOutput})

	responses := []*provider.GenerateResponse{
		{
			Message: &schema.Message{
				Role: schema.RoleAssistant,
				ToolCalls: []schema.ToolCall{{
					ID:        "call_big_1",
					Name:      "big_dump",
					Arguments: json.RawMessage(`{}`),
				}},
			},
		},
		{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: "done"},
		},
	}
	p := &sequencedProvider{responses: responses}

	eng := NewAgentEngine(p, registry, workDir, staticComposer{}, Config{MaxTurns: 4})

	if _, err := eng.RunWithReporter(context.Background(), sess, "fetch big", nil); err != nil {
		t.Fatalf("RunWithReporter() error = %v", err)
	}

	persistedPath := filepath.Join(sess.ToolResultsDir(), "call_big_1.txt")
	data, err := os.ReadFile(persistedPath)
	if err != nil {
		t.Fatalf("expected persisted tool result at %s: %v", persistedPath, err)
	}
	if len(data) != len(largeOutput) {
		t.Fatalf("persisted file size = %d, want %d", len(data), len(largeOutput))
	}

	messages, err := session.NewMessageLog(sess).LoadMessages()
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}
	var toolMsg *schema.Message
	for i := range messages {
		if messages[i].ToolCallID == "call_big_1" {
			toolMsg = &messages[i]
			break
		}
	}
	if toolMsg == nil {
		t.Fatalf("expected tool result message in history")
	}
	if !strings.Contains(toolMsg.Content, "<persisted-output>") {
		t.Fatalf("tool result in context should be preview, got: %q", toolMsg.Content[:200])
	}
	if len(toolMsg.Content) >= len(largeOutput) {
		t.Fatalf("preview should be smaller than full output: got %d, full %d", len(toolMsg.Content), len(largeOutput))
	}
}

func TestRun_BlocksWhenContextExceedsBlockingThreshold(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("session.Create: %v", err)
	}

	prov := &usageReportingProvider{content: "done"}
	reg := tools.NewRegistry()
	eng := NewAgentEngine(prov, reg, workDir, staticComposer{}, DefaultConfig())

	cfg := compaction.DefaultCompactionConfig()
	cfg.Model = "test"
	cfg.ContextWindow = 100
	cfg.Estimator = compaction.ImprovedRoughEstimator{}
	c, err := compaction.NewCompactor(prov, cfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	eng.WithCompactor(c)

	longPrompt := strings.Repeat("x", 2000)
	_, runErr := eng.Run(context.Background(), sess, longPrompt)
	if runErr == nil {
		t.Fatalf("expected error when context exceeds blocking threshold")
	}
	if !strings.Contains(runErr.Error(), "阻塞阈值") {
		t.Fatalf("unexpected error message: %v", runErr)
	}
}

func TestRun_ReportsContextEstimate(t *testing.T) {
	workDir := t.TempDir()
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("session.Create: %v", err)
	}

	prov := &usageReportingProvider{content: "done"}
	reg := tools.NewRegistry()

	var gotUsed, gotWindow int
	cfg := DefaultConfig()
	cfg.OnContextEstimate = func(usedTokens, contextWindow int) {
		gotUsed = usedTokens
		gotWindow = contextWindow
	}

	eng := NewAgentEngine(prov, reg, workDir, staticComposer{}, cfg)
	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = "test"
	compCfg.ContextWindow = 200000
	c, err := compaction.NewCompactor(prov, compCfg)
	if err != nil {
		t.Fatalf("NewCompactor: %v", err)
	}
	eng.WithCompactor(c)

	_, runErr := eng.Run(context.Background(), sess, "hello")
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if gotUsed == 0 {
		t.Fatalf("OnContextEstimate was not called; gotUsed = 0")
	}
	if gotWindow != 200000 {
		t.Fatalf("OnContextEstimate got contextWindow = %d, want 200000", gotWindow)
	}
}
