package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* failingPromptCollector answers one fixed collection failure. */
type failingPromptCollector struct {
	err error
}

func (c failingPromptCollector) Collect(context.Context, ContextCollectionRequest) ([]prompt.Fragment, error) {
	return nil, c.err
}

/* TestContextControllerKeepsBaselinePromptCompositionChain pins the baseline
 * prompt-composition failure chain: the composition error surfaces unwrapped,
 * the way the baseline composed the system prompt before assembling context. */
func TestContextControllerKeepsBaselinePromptCompositionChain(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	controller, _ := agentSession.NewContextController(scope, failingPromptCollector{err: errors.New("组装系统提示词失败: 读取 AGENTS.md 失败: unavailable")}, nil)
	_, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
	if err == nil || err.Error() != "组装系统提示词失败: 读取 AGENTS.md 失败: unavailable" {
		t.Fatalf("Prepare() error = %v, want the bare baseline composition chain", err)
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerKeepsBaselineCompactStateChains pins the baseline
 * compact-state chains: a durable-decision state load or save failure
 * surfaces inside the baseline assembly chain. */
func TestContextControllerKeepsBaselineCompactStateChains(t *testing.T) {
	t.Run("initial state load", func(t *testing.T) {
		store := newLifecycleStore()
		harness, _ := NewRuntimeHarness(store)
		agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
		store.failNextCompactLoad(errors.New("compact state unavailable"))
		scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
		controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
		_, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
		if err == nil || err.Error() != "组装 Session 上下文失败: compact state unavailable" {
			t.Fatalf("Prepare() error = %v, want the baseline assembly chain", err)
		}
		_ = agentSession.FinishRun(scope)
	})
	t.Run("initial state save", func(t *testing.T) {
		store := newLifecycleStore()
		harness, _ := NewRuntimeHarness(store)
		agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
		id := agentSession.Snapshot().ID
		store.seedMessages(id, []session.MessageRecord{
			{Seq: 0, Message: engine.Message{Role: engine.RoleUser, Content: "covered"}},
		})
		store.failNextCompactSave(errors.New("compact state unwritable"))
		scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
		compactor := contextCompactorFunc(func(_ context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
			if request.Trigger == ContextCompactionInitialHistory {
				return ContextCompactionProposal{
					Changed:      true,
					CompactState: &session.CompactState{Summary: "summary", CoveredUntilSeq: 0},
				}, nil
			}
			return ContextCompactionProposal{}, nil
		})
		controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
		_, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
		if err == nil || err.Error() != "组装 Session 上下文失败: compact state unwritable" {
			t.Fatalf("Prepare() error = %v, want the baseline assembly chain", err)
		}
		_ = agentSession.FinishRun(scope)
	})
}
