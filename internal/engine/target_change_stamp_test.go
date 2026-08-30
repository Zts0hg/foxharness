package engine

import (
	"context"
	"slices"
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

/* sourceRecordingConversation records each change source and content it receives. */
type sourceRecordingConversation struct {
	sources  []ConversationChangeSource
	contents []string
}

func (c *sourceRecordingConversation) Prepare(_ context.Context, request ConversationRequest) (ConversationProjection, error) {
	return ConversationProjection{Context: RunContext{
		Messages: []schema.Message{{Role: schema.RoleUser, Content: request.Input.Prompt}},
	}}, nil
}

func (c *sourceRecordingConversation) RequestChanges(_ context.Context, changes []ConversationChange) error {
	for _, change := range changes {
		c.sources = append(c.sources, change.Source)
		c.contents = append(c.contents, change.Message.Content)
	}
	return nil
}

/* TestTargetEngineSkipsEmptyThinkingResponse pins the baseline thinking guard:
 * the freeze point appended the thinking response to the conversation only
 * when it carried content, so an empty thinking message never reaches the
 * action projection. */
func TestTargetEngineSkipsEmptyThinkingResponse(t *testing.T) {
	conversation := &sourceRecordingConversation{}
	responses := []ModelResult{
		{Message: schema.Message{Role: schema.RoleAssistant, Content: ""}},
		{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}},
	}
	calls := 0
	model := modelInvokerFunc(func(_ context.Context, runContext RunContext) (ModelResult, error) {
		result := responses[min(calls, len(responses)-1)]
		calls++
		return result, nil
	})
	_, _ = NewAgentEngine(model, &recordingTargetToolExecutor{}, conversation, stampRecordingPolicy{}, nil).
		Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 3, Thinking: true})
	for index, source := range conversation.sources {
		if source == ConversationSourceThinking && conversation.contents[index] == "" {
			t.Fatalf("empty thinking response reached the conversation: %v/%v", conversation.sources, conversation.contents)
		}
	}
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

/* compactingProjectionConversation reports one committed compaction on every
 * ordinary prepare. */
type compactingProjectionConversation struct{}

func (c *compactingProjectionConversation) Prepare(_ context.Context, request ConversationRequest) (ConversationProjection, error) {
	return ConversationProjection{
		Context:     RunContext{Messages: []schema.Message{{Role: schema.RoleUser, Content: request.Input.Prompt}}},
		Compactions: []ConversationCompaction{{Trigger: "turn_context", BeforeMessages: 9, AfterMessages: 3}},
	}, nil
}

func (c *compactingProjectionConversation) RequestChanges(context.Context, []ConversationChange) error {
	return nil
}

/* TestTargetEngineRecordsCompactionBeforeTurnNotices pins the baseline
 * transcript order: a turn that both compacts and injects records the
 * context_compacted fact before the turn's injection facts, because the
 * baseline compacted before appending its reminders even though the changes
 * reach the conversation ahead of the preparation. */
func TestTargetEngineRecordsCompactionBeforeTurnNotices(t *testing.T) {
	var kinds []FactKind
	eng := NewAgentEngine(
		modelInvokerFunc(func(_ context.Context, _ RunContext) (ModelResult, error) {
			return ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "progress"}}, nil
		}),
		&recordingTargetToolExecutor{}, &compactingProjectionConversation{},
		stampRecordingPolicy{}, observerFunc(func(_ context.Context, fact Fact) {
			if fact.Kind == FactContextCompacted || fact.Kind == FactSystemReminder {
				kinds = append(kinds, fact.Kind)
			}
		}),
	)
	eng.Run(context.Background(), RunInput{Prompt: "work", MaxTurns: 2})
	/* Each turn records its compaction first, then the turn's injection
	 * facts. */
	want := []FactKind{
		FactContextCompacted, FactSystemReminder, FactSystemReminder,
		FactContextCompacted, FactSystemReminder, FactSystemReminder,
	}
	if !slices.Equal(kinds, want) {
		t.Fatalf("recorded fact order = %v, want %v", kinds, want)
	}
}
