package runtime

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* TestContextControllerAdmitsTurnOneInjectionsBeforeTheFirstPrepare pins the
 * baseline admission order: the engine requests the turn's policy notices
 * before the turn's first prepare, so a notice arriving before the run's
 * initial prepare must be accepted, kept out of the durable compaction input,
 * and carried on top of the compacted projection. */
func TestContextControllerAdmitsTurnOneInjectionsBeforeTheFirstPrepare(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	id := agentSession.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, Message: engine.Message{Role: engine.RoleUser, Content: "covered"}},
		{Seq: 1, Message: engine.Message{Role: engine.RoleAssistant, Content: "done"}},
	})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if err := controller.RequestChanges(context.Background(), []engine.ConversationChange{{
		Kind: engine.ConversationAppendContextMessage, Source: engine.ConversationSourceNextTurnReminder,
		Message: engine.Message{Role: engine.RoleUser, Content: "queued activation"},
	}}); err != nil {
		t.Fatalf("turn-one notice before the first Prepare() = %v, want accepted", err)
	}
	projection, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
	if err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) != 2 || compactor.requests[0].Trigger != ContextCompactionInitialHistory {
		t.Fatalf("compaction requests = %#v, want the turn-one durable and run-local decisions", compactor.requests)
	}
	for _, request := range compactor.requests {
		if slices.Contains(messageContents(request.Messages), "queued activation") {
			t.Fatalf("compaction input carried the turn-one notice: %#v", request.Messages)
		}
	}
	if !slices.Contains(messageContents(projection.Context.Messages), "queued activation") {
		t.Fatalf("projection lost the turn-one notice: %v", messageContents(projection.Context.Messages))
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerTurnOneWithoutThinkingKeepsBothDecisionPoints pins the
 * baseline turn shape on the default no-thinking path: the single first-turn
 * prepare runs the durable session-history decision and still runs the
 * run-local decision the baseline executed at every turn start. */
func TestContextControllerTurnOneWithoutThinkingKeepsBothDecisionPoints(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	request := ordinaryConversationRequest("work")
	request.Phase = engine.PhaseAction
	if _, err := controller.Prepare(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	got := compactionTriggers(compactor)
	want := []ContextCompactionTrigger{ContextCompactionInitialHistory, ContextCompactionPreTurn}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("no-thinking turn 1 compaction triggers = %v, want %v", got, want)
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerTurnOneThinkingKeepsPrePrepareNoticesOutOfRunLocal pins
 * the baseline first-turn shape with thinking: the notices the engine
 * requested before the thinking prepare stay out of the run-local decision's
 * input, because the baseline appended its turn-one reminders after both
 * first-turn compaction decisions had run. */
func TestContextControllerTurnOneThinkingKeepsPrePrepareNoticesOutOfRunLocal(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if err := controller.RequestChanges(context.Background(), []engine.ConversationChange{{
		Kind: engine.ConversationAppendContextMessage, Source: engine.ConversationSourceReminder, Turn: 1,
		Message: engine.Message{Role: engine.RoleUser, Content: "turn-one pre-prepare notice"},
	}}); err != nil {
		t.Fatal(err)
	}
	thinking := ordinaryConversationRequest("work")
	thinking.Phase = engine.PhaseThinking
	if _, err := controller.Prepare(context.Background(), thinking); err != nil {
		t.Fatal(err)
	}
	action := ordinaryConversationRequest("work")
	projection, err := controller.Prepare(context.Background(), action)
	if err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) < 2 {
		t.Fatalf("compaction requests = %#v", compactor.requests)
	}
	if slices.Contains(messageContents(compactor.requests[1].Messages), "turn-one pre-prepare notice") {
		t.Fatalf("run-local input carried the pre-prepare notice: %#v", compactor.requests[1].Messages)
	}
	if !slices.Contains(messageContents(projection.Context.Messages), "turn-one pre-prepare notice") {
		t.Fatalf("projection lost the pre-prepare notice: %v", messageContents(projection.Context.Messages))
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerPreTurnCompactionKeepsPreviousTurnNotices pins the
 * baseline pre-turn compaction input: it covers everything appended through
 * the end of the previous turn — including the previous turn's gate
 * reminders — and excludes only the notices the current turn produced. */
func TestContextControllerPreTurnCompactionKeepsPreviousTurnNotices(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if _, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work")); err != nil {
		t.Fatal(err)
	}
	/* The previous turn's completion-gate reminder lands after that turn's
	 * last prepare; the current turn's notices arrive before its first one. */
	if err := controller.RequestChanges(context.Background(), []engine.ConversationChange{
		{
			Kind: engine.ConversationAppendContextMessage, Source: engine.ConversationSourceCompletionGate,
			Turn:    1,
			Message: engine.Message{Role: engine.RoleUser, Content: "previous turn gate reminder"},
		},
		{
			Kind: engine.ConversationAppendContextMessage, Source: engine.ConversationSourceReminder,
			Turn:    2,
			Message: engine.Message{Role: engine.RoleUser, Content: "current turn reminder"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	request := ordinaryConversationRequest("work")
	request.Turn = 2
	projection, err := controller.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) < 3 {
		t.Fatalf("compaction requests = %#v", compactor.requests)
	}
	/* The turn-two prepare carries the run-local decision, after the
	 * no-thinking turn one consumed both of its decision points. */
	input := messageContents(compactor.requests[2].Messages)
	if !slices.Contains(input, "previous turn gate reminder") {
		t.Fatalf("pre-turn compaction input lost the previous turn's notice: %v", input)
	}
	if slices.Contains(input, "current turn reminder") {
		t.Fatalf("pre-turn compaction input carried the current turn's notice: %v", input)
	}
	output := messageContents(projection.Context.Messages)
	if !slices.Contains(output, "previous turn gate reminder") || !slices.Contains(output, "current turn reminder") {
		t.Fatalf("projection lost a notice: %v", output)
	}
	_ = agentSession.FinishRun(scope)
}
