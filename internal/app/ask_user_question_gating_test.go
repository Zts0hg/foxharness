package app

import (
	"context"
	"testing"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// gatingFakeAsker is a no-op UserAsker used only to flip the gating condition.
type gatingFakeAsker struct{}

func (gatingFakeAsker) Ask(ctx context.Context, questions []tools.Question) ([]tools.Answer, error) {
	return nil, nil
}

func registryHasTool(reg tools.Registry, name string) bool {
	for _, def := range reg.GetAvailableTools() {
		if def.Name == name {
			return true
		}
	}
	return false
}

func TestBuildRegistryGatesAskUserQuestion(t *testing.T) {
	t.Setenv("ZHIPU_API_KEY", "test-key")
	workDir := t.TempDir()
	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		t.Fatalf("EnsureFiles() error = %v", err)
	}
	manager := session.NewManagerWithHome(workDir, t.TempDir())
	sess, err := manager.Create(session.CreateOptions{Source: session.SOURCECLI, WorkDir: workDir})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	llm := &blockingLLMProvider{entered: make(chan struct{}), release: make(chan struct{})}
	cp := checkpoint.New(checkpoint.Config{SessionDir: sess.RootDir})
	getID := func() string { return "" }

	runner := &AgentRunner{
		workDir:        workDir,
		store:          store,
		manager:        manager,
		llmProvider:    llm,
		currentSession: sess,
	}

	// Without an asker (the fox exec / agentops / feishu / bench / subagent case)
	// the tool must NOT be exposed to the model.
	reg := runner.buildRegistry(sess, llm, cp, getID)
	if registryHasTool(reg, "ask_user_question") {
		t.Fatal("ask_user_question must be absent when no asker is set")
	}

	// With an asker set (the TUI case) the tool must be present.
	runner.SetUserAsker(gatingFakeAsker{})
	regWithAsker := runner.buildRegistry(sess, llm, cp, getID)
	if !registryHasTool(regWithAsker, "ask_user_question") {
		t.Fatal("ask_user_question must be present when an asker is set")
	}
}
