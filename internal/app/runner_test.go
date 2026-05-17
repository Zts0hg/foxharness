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
	manager := session.NewManager(workDir)
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
