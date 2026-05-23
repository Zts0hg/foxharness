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
}

func (p *sequencedProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	idx := p.call
	if idx >= len(p.responses) {
		idx = len(p.responses) - 1
	}
	p.call++
	return p.responses[idx], nil
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
