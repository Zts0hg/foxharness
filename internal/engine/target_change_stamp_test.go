package engine

import (
	"context"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

/* stampRecordingConversation records the producing turn each change carries
 * when it reaches the conversation. */
type stampRecordingConversation struct {
	stamps []int
}

func (c *stampRecordingConversation) Prepare(_ context.Context, request ConversationRequest) (ConversationProjection, error) {
	return ConversationProjection{Context: RunContext{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: request.Input.Prompt}},
	}}, nil
}

func (c *stampRecordingConversation) RequestChanges(_ context.Context, changes []ConversationChange) error {
	for _, change := range changes {
		c.stamps = append(c.stamps, change.Turn)
	}
	return nil
}

/* stampRecordingPolicy produces a BeforeTurn notice on every turn and an
 * AfterModel gate reminder whenever the model answers without tool calls. */
type stampRecordingPolicy struct{}

func (stampRecordingPolicy) StartRun(context.Context, RunInput) (TurnRunPolicy, error) {
	return stampRecordingRunPolicy{}, nil
}

type stampRecordingRunPolicy struct{}

func (stampRecordingRunPolicy) BeforeTurn(_ context.Context, _ TurnState) (PolicyChanges, error) {
	return PolicyChanges{Changes: []ConversationChange{{
		Kind: ConversationAppendContextMessage, Source: ConversationSourceReminder,
		Message: schema.Message{Role: schema.RoleUser, Content: "turn notice"},
	}}}, nil
}

func (stampRecordingRunPolicy) AfterModel(context.Context, TurnState) (TurnDecision, error) {
	return TurnDecision{Changes: []ConversationChange{{
		Kind: ConversationAppendContextMessage, Source: ConversationSourceCompletionGate,
		Message: schema.Message{Role: schema.RoleUser, Content: "gate reminder"},
	}}}, nil
}

func (stampRecordingRunPolicy) AfterTools(context.Context, ToolState) (PolicyChanges, error) {
	return PolicyChanges{}, nil
}

/* TestTargetEngineStampsConversationChangesWithTheProducingTurn pins the
 * attribution the compaction controller needs: every change the engine hands
 * to the conversation carries the turn that produced it, so the pre-turn
 * compaction can exclude the producing turn's notices while keeping the
 * previous turn's. */
func TestTargetEngineStampsConversationChangesWithTheProducingTurn(t *testing.T) {
	conversation := &stampRecordingConversation{}
	complete := modelInvokerFunc(func(_ context.Context, _ RunContext) (ModelResult, error) {
		return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "progress"}}, nil
	})
	NewAgentEngine(complete, &recordingTargetToolExecutor{}, conversation, stampRecordingPolicy{}, nil).
		Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2})
	/* Each turn contributes its BeforeTurn notice, its committed assistant
	 * message, and its AfterModel gate reminder, all stamped with that turn. */
	want := []int{1, 1, 1, 2, 2, 2}
	if len(conversation.stamps) != len(want) {
		t.Fatalf("change stamps = %v, want %v", conversation.stamps, want)
	}
	for index, stamp := range conversation.stamps {
		if stamp != want[index] {
			t.Fatalf("change stamps = %v, want %v", conversation.stamps, want)
		}
	}
}
