package app

import (
	"context"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

type blockingLLMProvider struct {
	entered chan struct{}
	release chan struct{}
}

func (p *blockingLLMProvider) Generate(ctx context.Context, messages []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error) {
	select {
	case <-p.entered:
	default:
		close(p.entered)
	}

	select {
	case <-p.release:
		return &schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil
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
	if err := session.NewMessageLog(sess).Append("run-1", schema.Message{Role: schema.RoleUser, Content: "hello"}); err != nil {
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
	if err := session.NewMessageLog(sess).Append("run-1", schema.Message{Role: schema.RoleUser, Content: "restore me"}); err != nil {
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
