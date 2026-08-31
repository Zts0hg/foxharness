package engine

import (
	"context"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

/* TestTargetRunKeepsLastNonEmptyAssistantTextAfterPureToolTurn pins the
 * terminal result to the most recent non-empty assistant text: a final turn
 * that only requests tools must not erase it. */
func TestTargetRunKeepsLastNonEmptyAssistantTextAfterPureToolTurn(t *testing.T) {
	invoker := toolCallThenContentInvoker(
		ModelResult{
			Message: schema.Message{
				Role: schema.RoleAssistant, Content: "real answer",
				ToolCalls: []schema.ToolCall{{ID: "call-one", Name: "inspect"}},
			},
			FinishReason: "tool_calls",
		},
		ModelResult{
			Message: schema.Message{
				Role:      schema.RoleAssistant,
				ToolCalls: []schema.ToolCall{{ID: "call-two", Name: "inspect"}},
			},
			FinishReason: "tool_calls",
		},
	)
	eng := NewAgentEngine(invoker, &turnBoundaryTargetToolExecutor{}, &targetTestConversation{}, targetTestPolicy{}, nil)

	outcome, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2})
	if err == nil {
		t.Fatalf("Run() error = nil, want turn limit failure")
	}
	if outcome.FinalMessage != "real answer" {
		t.Fatalf("Run() FinalMessage = %q, want the last non-empty assistant text %q", outcome.FinalMessage, "real answer")
	}
}

/* TestTargetRunFinalMessageTracksLatestAssistantText pins the terminal result
 * to the newest assistant text whenever the closing turn produces one. */
func TestTargetRunFinalMessageTracksLatestAssistantText(t *testing.T) {
	invoker := toolCallThenContentInvoker(
		ModelResult{
			Message: schema.Message{
				Role: schema.RoleAssistant, Content: "intermediate",
				ToolCalls: []schema.ToolCall{{ID: "call-one", Name: "inspect"}},
			},
			FinishReason: "tool_calls",
		},
		ModelResult{
			Message:      schema.Message{Role: schema.RoleAssistant, Content: "final answer"},
			FinishReason: "stop",
		},
	)
	eng := NewAgentEngine(invoker, &turnBoundaryTargetToolExecutor{}, &targetTestConversation{}, targetTestPolicy{}, nil)

	outcome, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if outcome.FinalMessage != "final answer" {
		t.Fatalf("Run() FinalMessage = %q, want %q", outcome.FinalMessage, "final answer")
	}
}

/* TestTargetRunEmitsInjectionFactsForPolicyContext pins the canonical fact
 * stream: policy-injected reminders and recovery notices become observable
 * facts in canonical order around the turn that injected them. */
func TestTargetRunEmitsInjectionFactsForPolicyContext(t *testing.T) {
	var observed []FactKind
	invocations := 0
	invoker := modelInvokerFunc(func(context.Context, RunContext) (ModelResult, error) {
		invocations++
		if invocations == 1 {
			return ModelResult{Message: schema.Message{
				Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "call-one", Name: "inspect"}},
			}}, nil
		}
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop"}, nil
	})
	policy := &injectionTestPolicy{reminder: "reminder body", recovery: "recovery body"}
	eng := NewAgentEngine(
		invoker,
		&turnBoundaryTargetToolExecutor{},
		&targetTestConversation{},
		policy,
		observerFunc(func(_ context.Context, fact Fact) { observed = append(observed, fact.Kind) }),
	)
	if _, err := eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []FactKind{
		FactRunStarted, FactSystemReminder, FactToolCall, FactToolResult,
		FactErrorRecovery, FactSystemReminder, FactMessage, FactRunCompleted,
	}
	if got := len(observed); got != len(want) {
		t.Fatalf("observed fact kinds = %#v, want %d facts", observed, len(want))
	}
	for index, kind := range want {
		if observed[index] != kind {
			t.Fatalf("fact[%d] = %q, want %q (all: %#v)", index, observed[index], kind, observed)
		}
	}
}

/* injectionTestPolicy injects one reminder before the turn and one recovery
 * notice after the tool round. */
type injectionTestPolicy struct {
	reminder string
	recovery string
}

func (p *injectionTestPolicy) StartRun(context.Context, RunInput) (TurnRunPolicy, error) {
	return p, nil
}

func (p *injectionTestPolicy) BeforeTurn(context.Context, TurnState) (PolicyChanges, error) {
	return PolicyChanges{Changes: []ConversationChange{
		{Kind: ConversationAppendContextMessage, Source: ConversationSourceReminder, Message: schema.Message{Role: RoleUser, Content: "[Runtime System Reminder]\n\n" + p.reminder}},
	}}, nil
}

func (p *injectionTestPolicy) AfterModel(context.Context, TurnState) (TurnDecision, error) {
	return TurnDecision{Complete: true}, nil
}

func (p *injectionTestPolicy) AfterTools(context.Context, ToolState) (PolicyChanges, error) {
	return PolicyChanges{Changes: []ConversationChange{
		{Kind: ConversationAppendContextMessage, Source: ConversationSourceRecovery, Message: schema.Message{Role: RoleUser, Content: "[Runtime System Notice]\n\n" + p.recovery}},
	}}, nil
}

/* toolCallThenContentInvoker scripts consecutive model results for one run. */
func toolCallThenContentInvoker(steps ...ModelResult) ModelInvoker {
	index := 0
	return modelInvokerFunc(func(context.Context, RunContext) (ModelResult, error) {
		if index >= len(steps) {
			return ModelResult{}, nil
		}
		step := steps[index]
		index++
		return step, nil
	})
}
