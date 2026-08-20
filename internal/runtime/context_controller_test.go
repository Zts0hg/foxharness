package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestContextControllerCollectsAndCommitsInitialContextOnce(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{
		Prompt: "model prompt", DisplayPrompt: "/skill visible", Model: "model-a",
		ProviderProtocol: "responses", Effort: "high", CollaborationMode: "default",
		AllowedTools: []string{"read_file"},
	})
	collector := &recordingContextCollector{fragments: []prompt.Fragment{
		prompt.Text("base"), prompt.Section("Project Instructions", "inspect first"),
	}}
	controller, err := agentSession.NewContextController(scope, collector, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := engine.ConversationRequest{
		Input: engine.RunInput{Prompt: "model prompt"}, Turn: 1, Phase: engine.PhaseAction,
		ToolDefinitions: []engine.ToolDefinition{{Name: "read_file"}},
		Preparation:     engine.ConversationPrepareNormal,
	}
	first, err := controller.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := controller.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	wantMessages := []engine.Message{
		{Role: engine.RoleSystem, Content: "base\n\n## Project Instructions\n\ninspect first"},
		{Role: engine.RoleUser, Content: "model prompt"},
	}
	if !reflect.DeepEqual(first.Context.Messages, wantMessages) || !reflect.DeepEqual(second.Context.Messages, wantMessages) {
		t.Fatalf("projections = %#v / %#v, want %#v", first.Context.Messages, second.Context.Messages, wantMessages)
	}
	if first.Context.Model != "model-a" || first.Context.Provider != "responses" || first.Context.Effort != "high" {
		t.Fatalf("run snapshot = %#v", first)
	}
	if collector.calls != 1 {
		t.Fatalf("collector calls = %d, want 1", collector.calls)
	}
	records := store.messageRecords(agentSession.Snapshot().ID)
	if len(records) != 1 || records[0].RunID != scope.Snapshot().RunID || records[0].Message.Content != "model prompt" || records[0].DisplayContent != "/skill visible" {
		t.Fatalf("initial records = %#v", records)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerInitialMessageAuthoritySurvivesControllerReplacement(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})

	first, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
	if _, err := first.Prepare(context.Background(), ordinaryConversationRequest("work")); err != nil {
		t.Fatal(err)
	}
	second, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
	projection, err := second.Prepare(context.Background(), ordinaryConversationRequest("work"))
	if err != nil {
		t.Fatal(err)
	}
	if got := recordContents(store.messageRecords(agentSession.Snapshot().ID)); !reflect.DeepEqual(got, []string{"work"}) {
		t.Fatalf("persisted messages = %v, want one initial user message", got)
	}
	if got := messageContents(projection.Context.Messages); !reflect.DeepEqual(got, []string{"system", "work"}) {
		t.Fatalf("replacement projection = %v", got)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerCompactsAtMostOncePerTurn(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)

	first := ordinaryConversationRequest("work")
	first.Phase = engine.PhaseThinking
	if _, err := controller.Prepare(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	first.Phase = engine.PhaseAction
	if _, err := controller.Prepare(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	second := ordinaryConversationRequest("work")
	second.Turn = 2
	if _, err := controller.Prepare(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	got := []ContextCompactionTrigger{}
	for _, request := range compactor.requests {
		got = append(got, request.Trigger)
	}
	want := []ContextCompactionTrigger{ContextCompactionInitialHistory, ContextCompactionPreTurn}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compaction triggers = %v, want %v", got, want)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerUsesExactPersistedRecordMetadataWhileLive(t *testing.T) {
	store := newLifecycleStore()
	fixed := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	store.messageTime = fixed
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if _, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work")); err != nil {
		t.Fatal(err)
	}
	request := ordinaryConversationRequest("work")
	request.Turn = 2
	if _, err := controller.Prepare(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) != 2 || len(compactor.requests[1].Records) != 1 {
		t.Fatalf("compactor requests = %#v", compactor.requests)
	}
	if got := compactor.requests[1].Records[0].Time; !got.Equal(fixed) {
		t.Fatalf("live record time = %v, want persisted %v", got, fixed)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerPreTurnCompactionFailureFallsBackToOriginalProjection(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := contextCompactorFunc(func(_ context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
		if request.Trigger == ContextCompactionPreTurn {
			return ContextCompactionProposal{}, errors.New("summary provider failed")
		}
		return ContextCompactionProposal{}, nil
	})
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if _, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work")); err != nil {
		t.Fatal(err)
	}
	request := ordinaryConversationRequest("work")
	request.Turn = 2
	projection, err := controller.Prepare(context.Background(), request)
	if err != nil {
		t.Fatalf("pre-turn fallback error = %v", err)
	}
	if got := messageContents(projection.Context.Messages); !reflect.DeepEqual(got, []string{"system", "work"}) {
		t.Fatalf("fallback projection = %v", got)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerRejectsCompactionProposalWithTwoAuthorities(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{proposal: ContextCompactionProposal{
		Changed:      true,
		Messages:     []engine.Message{{Role: engine.RoleUser, Content: "ephemeral"}},
		CompactState: &session.CompactState{Summary: "durable", CoveredUntilSeq: 0},
	}}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if _, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work")); err == nil || !strings.Contains(err.Error(), "two authorities") {
		t.Fatalf("Prepare() error = %v, want two-authority rejection", err)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerBlockingDecisionStopsBeforeModelProjection(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := contextCompactorFunc(func(context.Context, ContextCompactionRequest) (ContextCompactionProposal, error) {
		return ContextCompactionProposal{}, &ContextBlockedError{UsedTokens: 101, Limit: 100}
	})
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	_, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
	var blocked *ContextBlockedError
	if !errors.As(err, &blocked) || blocked.UsedTokens != 101 || blocked.Limit != 100 {
		t.Fatalf("Prepare() error = %v, want typed blocking decision", err)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerChecksPostThinkingActionBudgetWithSameToolSnapshot(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	thinking := true
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work", Thinking: &thinking})
	compactor := &budgetCheckingCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	request := ordinaryConversationRequest("work")
	request.Phase = engine.PhaseThinking
	request.ToolDefinitions = []engine.ToolDefinition{{Name: "inspect"}}
	if _, err := controller.Prepare(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := controller.RequestChanges(context.Background(), []engine.ConversationChange{{
		Kind: engine.ConversationAppendContextMessage, Source: engine.ConversationSourceThinking,
		Message: engine.Message{Role: engine.RoleAssistant, Content: "large thinking"},
	}}); err != nil {
		t.Fatal(err)
	}
	request.Phase = engine.PhaseAction
	_, err := controller.Prepare(context.Background(), request)
	var blocked *ContextBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("action Prepare() error = %v, want post-thinking blocking decision", err)
	}
	if len(compactor.checks) != 1 || len(compactor.checks[0].ToolDefinitions) != 1 || compactor.checks[0].ToolDefinitions[0].Name != "inspect" {
		t.Fatalf("budget checks = %#v", compactor.checks)
	}
	if got := messageContents(compactor.checks[0].Messages); !reflect.DeepEqual(got, []string{"system", "work", "large thinking"}) {
		t.Fatalf("budget messages = %v", got)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerPreservesPersistedAndTransientChangeOrder(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
	_, _ = controller.Prepare(context.Background(), ordinaryConversationRequest("work"))

	changes := []engine.ConversationChange{
		{Kind: engine.ConversationAppendMessage, Message: engine.Message{Role: engine.RoleAssistant, Content: "assistant"}},
		{Kind: engine.ConversationAppendContextMessage, Source: engine.ConversationSourceReminder, Message: engine.Message{Role: engine.RoleUser, Content: "reminder"}},
		{Kind: engine.ConversationAppendMessage, Message: engine.Message{Role: engine.RoleUser, Content: "tool result", ToolCallID: "call-1"}},
	}
	if err := controller.RequestChanges(context.Background(), changes); err != nil {
		t.Fatal(err)
	}
	changes[0].Message.Content = "mutated"
	projected, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"system", "work", "assistant", "reminder", "tool result"}
	if got := messageContents(projected.Context.Messages); !reflect.DeepEqual(got, want) {
		t.Fatalf("projected order = %v, want %v", got, want)
	}
	records := store.messageRecords(agentSession.Snapshot().ID)
	if got := recordContents(records); !reflect.DeepEqual(got, []string{"work", "assistant", "tool result"}) {
		t.Fatalf("persisted order = %v", got)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerResumesFromPersistedCompactState(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	created, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	id := created.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, RunID: "old", Message: engine.Message{Role: engine.RoleUser, Content: "covered user"}},
		{Seq: 1, RunID: "old", Message: engine.Message{Role: engine.RoleAssistant, Content: "covered answer"}},
		{Seq: 2, RunID: "old", Message: engine.Message{Role: engine.RoleUser, Content: "active user"}},
	})
	store.seedCompactState(id, &session.CompactState{Summary: "earlier summary", CoveredUntilSeq: 1})
	_ = created.Close(context.Background())

	opened, _ := harness.OpenSession(context.Background(), CLIExec, id)
	scope, _ := opened.BeginRun(context.Background(), RunSpec{Prompt: "continue"})
	controller, _ := opened.NewContextController(scope, staticContextCollector("system"), nil)
	projection, err := controller.Prepare(context.Background(), ordinaryConversationRequest("continue"))
	if err != nil {
		t.Fatal(err)
	}
	contents := messageContents(projection.Context.Messages)
	if len(contents) != 4 || contents[0] != "system" || contents[2] != "active user" || contents[3] != "continue" {
		t.Fatalf("resumed projection = %#v", contents)
	}
	if got := contents[1]; got == "" || !containsAll(got, "## Compacted Context Summary", "earlier summary", opened.Snapshot().RootDir+"/transcript.jsonl") {
		t.Fatalf("summary projection = %q", got)
	}
	_ = opened.FinishRun(scope)
}

func TestContextControllerCommitsCompactionProposalThroughAgentSession(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{proposal: ContextCompactionProposal{
		Changed:      true,
		CompactState: &session.CompactState{Summary: "summary", CoveredUntilSeq: 0},
	}}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	request := ordinaryConversationRequest("work")
	request.ToolDefinitions = []engine.ToolDefinition{{Name: "read_file"}}
	projection, err := controller.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.ToolDefinitions[0].Name = "mutated"
	if got := messageContents(projection.Context.Messages); len(got) != 2 || got[0] != "system" || !strings.Contains(got[1], "summary") {
		t.Fatalf("compacted projection = %v", got)
	}
	if len(compactor.requests) != 1 || compactor.requests[0].Trigger != ContextCompactionInitialHistory || compactor.requests[0].ToolDefinitions[0].Name != "read_file" {
		t.Fatalf("compaction request = %#v", compactor.requests)
	}
	state := store.compactState(agentSession.Snapshot().ID)
	if state == nil || state.Summary != "summary" || state.CoveredUntilSeq != 0 {
		t.Fatalf("persisted compact state = %#v", state)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerAuthoritativeWriteFailureDoesNotMutateProjection(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
	_, _ = controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
	store.failNextMessage(errors.New("message write failed"))
	change := []engine.ConversationChange{{
		Kind:    engine.ConversationAppendMessage,
		Message: engine.Message{Role: engine.RoleAssistant, Content: "answer"},
	}}
	if err := controller.RequestChanges(context.Background(), change); err == nil {
		t.Fatal("authoritative message failure was lost")
	}
	projection, _ := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
	if got := messageContents(projection.Context.Messages); !reflect.DeepEqual(got, []string{"system", "work"}) {
		t.Fatalf("failed write mutated projection: %v", got)
	}
	if err := controller.RequestChanges(context.Background(), change); err != nil {
		t.Fatal(err)
	}
	if got := recordContents(store.messageRecords(agentSession.Snapshot().ID)); !reflect.DeepEqual(got, []string{"work", "answer"}) {
		t.Fatalf("retry records = %v", got)
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerValidatesWholeChangeBatchBeforePersistence(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
	_, _ = controller.Prepare(context.Background(), ordinaryConversationRequest("work"))

	err := controller.RequestChanges(context.Background(), []engine.ConversationChange{
		{Kind: engine.ConversationAppendMessage, Message: engine.Message{Role: engine.RoleAssistant, Content: "must not persist"}},
		{Kind: engine.ConversationAppendContextMessage, Message: engine.Message{Role: engine.RoleUser, Content: "missing source"}},
	})
	if err == nil {
		t.Fatal("invalid batch error = nil")
	}
	if got := recordContents(store.messageRecords(agentSession.Snapshot().ID)); !reflect.DeepEqual(got, []string{"work"}) {
		t.Fatalf("records after invalid batch = %v", got)
	}
	_ = agentSession.FinishRun(scope)
}

func TestAgentSessionRewindContextInvalidatesCoveredSummaryAndSerializesWithRuns(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	id := agentSession.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, Message: engine.Message{Role: engine.RoleUser, Content: "zero"}},
		{Seq: 1, Message: engine.Message{Role: engine.RoleAssistant, Content: "one"}},
		{Seq: 2, Message: engine.Message{Role: engine.RoleUser, Content: "two"}},
	})
	store.seedCompactState(id, &session.CompactState{Summary: "summary", CoveredUntilSeq: 1})

	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "active"})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := agentSession.RewindContext(cancelled, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("rewind during run = %v, want context cancellation", err)
	}
	_ = agentSession.FinishRun(scope)
	if err := agentSession.RewindContext(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if got := recordContents(store.messageRecords(id)); !reflect.DeepEqual(got, []string{"zero"}) {
		t.Fatalf("rewound records = %v", got)
	}
	if state := store.compactState(id); state == nil || state.Summary != "" || state.CoveredUntilSeq != -1 {
		t.Fatalf("rewound compact state = %#v", state)
	}
}

func TestAgentSessionManualCompactionCommitsProposal(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	id := agentSession.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, Message: engine.Message{Role: engine.RoleUser, Content: "zero"}},
		{Seq: 1, Message: engine.Message{Role: engine.RoleAssistant, Content: "one"}},
	})
	compactor := &recordingContextCompactor{proposal: ContextCompactionProposal{
		Changed:      true,
		CompactState: &session.CompactState{Summary: "manual summary", CoveredUntilSeq: 1},
	}}
	proposal, err := agentSession.CompactContext(context.Background(), compactor, "focus on decisions")
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.Changed || len(compactor.requests) != 1 || compactor.requests[0].Trigger != ContextCompactionManual || compactor.requests[0].Instructions != "focus on decisions" {
		t.Fatalf("manual proposal/request = %#v / %#v", proposal, compactor.requests)
	}
	if state := store.compactState(id); state == nil || state.Summary != "manual summary" || state.CoveredUntilSeq != 1 {
		t.Fatalf("manual compact state = %#v", state)
	}
}

func TestAgentSessionManualCompactionSurvivesContinuationAndReopen(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	id := agentSession.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, RunID: "old", Message: engine.Message{Role: engine.RoleUser, Content: "zero"}},
		{Seq: 1, RunID: "old", Message: engine.Message{Role: engine.RoleAssistant, Content: "one"}},
	})
	compactor := &recordingContextCompactor{proposal: ContextCompactionProposal{
		Changed: true, CompactState: &session.CompactState{Summary: "manual summary", CoveredUntilSeq: 1},
	}}
	if _, err := agentSession.CompactContext(context.Background(), compactor, ""); err != nil {
		t.Fatal(err)
	}
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "continue"})
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
	projection, err := controller.Prepare(context.Background(), ordinaryConversationRequest("continue"))
	if err != nil {
		t.Fatal(err)
	}
	assertSingleSummaryProjection(t, projection.Context.Messages, "continue")
	_ = agentSession.FinishRun(scope)
	_ = agentSession.Close(context.Background())

	reopened, _ := harness.OpenSession(context.Background(), TUIInteractive, id)
	reopenedScope, _ := reopened.BeginRun(context.Background(), RunSpec{Prompt: "again"})
	reopenedController, _ := reopened.NewContextController(reopenedScope, staticContextCollector("system"), nil)
	reopenedProjection, err := reopenedController.Prepare(context.Background(), ordinaryConversationRequest("again"))
	if err != nil {
		t.Fatal(err)
	}
	contents := messageContents(reopenedProjection.Context.Messages)
	if len(contents) != 4 || contents[2] != "continue" || contents[3] != "again" {
		t.Fatalf("reopened projection = %v", contents)
	}
	assertSingleSummaryProjection(t, reopenedProjection.Context.Messages, "again")
	_ = reopened.FinishRun(reopenedScope)
}

func TestAgentSessionRewindNeverProjectsTruncatedFutureAcrossCompactCoverage(t *testing.T) {
	for _, testCase := range []struct {
		name         string
		seq          int64
		wantContents []string
		wantSummary  bool
	}{
		{name: "before coverage end", seq: 1, wantContents: []string{"system", "zero", "continue"}},
		{name: "at coverage end", seq: 2, wantContents: []string{"system", "zero", "one", "continue"}},
		{name: "after coverage", seq: 4, wantContents: []string{"system", "summary", "three", "continue"}, wantSummary: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			store := newLifecycleStore()
			harness, _ := NewRuntimeHarness(store)
			agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
			id := agentSession.Snapshot().ID
			store.seedMessages(id, []session.MessageRecord{
				{Seq: 0, RunID: "old", Message: engine.Message{Role: engine.RoleUser, Content: "zero"}},
				{Seq: 1, RunID: "old", Message: engine.Message{Role: engine.RoleAssistant, Content: "one"}},
				{Seq: 2, RunID: "old", Message: engine.Message{Role: engine.RoleUser, Content: "two"}},
				{Seq: 3, RunID: "old", Message: engine.Message{Role: engine.RoleAssistant, Content: "three"}},
				{Seq: 4, RunID: "old", Message: engine.Message{Role: engine.RoleUser, Content: "future"}},
			})
			store.seedCompactState(id, &session.CompactState{Summary: "summary", CoveredUntilSeq: 2})
			if err := agentSession.RewindContext(context.Background(), testCase.seq); err != nil {
				t.Fatal(err)
			}
			scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "continue"})
			controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), nil)
			projection, err := controller.Prepare(context.Background(), ordinaryConversationRequest("continue"))
			if err != nil {
				t.Fatal(err)
			}
			contents := messageContents(projection.Context.Messages)
			if testCase.wantSummary {
				if len(contents) != 4 || !strings.Contains(contents[1], "summary") {
					t.Fatalf("projection = %v", contents)
				}
				contents[1] = "summary"
			}
			if !reflect.DeepEqual(contents, testCase.wantContents) {
				t.Fatalf("projection = %v, want %v", contents, testCase.wantContents)
			}
			if strings.Contains(strings.Join(contents, "|"), "future") {
				t.Fatalf("truncated future reappeared: %v", contents)
			}
			_ = agentSession.FinishRun(scope)
		})
	}
}

func TestAgentSessionRejectsManualContextOperationsOutsideProfileCapabilities(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	compactor := &recordingContextCompactor{}
	if _, err := agentSession.CompactContext(context.Background(), compactor, ""); err == nil {
		t.Fatal("CLI manual compaction error = nil")
	}
	if err := agentSession.RewindContext(context.Background(), 0); err == nil {
		t.Fatal("CLI rewind error = nil")
	}
}

type recordingContextCollector struct {
	fragments []prompt.Fragment
	calls     int
}

func (c *recordingContextCollector) Collect(context.Context, ContextCollectionRequest) ([]prompt.Fragment, error) {
	c.calls++
	return append([]prompt.Fragment(nil), c.fragments...), nil
}

type staticContextCollector string

func (c staticContextCollector) Collect(context.Context, ContextCollectionRequest) ([]prompt.Fragment, error) {
	return []prompt.Fragment{prompt.Text(string(c))}, nil
}

type recordingContextCompactor struct {
	proposal ContextCompactionProposal
	requests []ContextCompactionRequest
}

type contextCompactorFunc func(context.Context, ContextCompactionRequest) (ContextCompactionProposal, error)

func (f contextCompactorFunc) Compact(ctx context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
	return f(ctx, request)
}

func (contextCompactorFunc) CheckContext(context.Context, ContextBudgetRequest) error {
	return nil
}

type budgetCheckingCompactor struct {
	checks []ContextBudgetRequest
}

func (*budgetCheckingCompactor) Compact(context.Context, ContextCompactionRequest) (ContextCompactionProposal, error) {
	return ContextCompactionProposal{}, nil
}

func (c *budgetCheckingCompactor) CheckContext(_ context.Context, request ContextBudgetRequest) error {
	c.checks = append(c.checks, request)
	if strings.Contains(strings.Join(messageContents(request.Messages), "|"), "large thinking") {
		return &ContextBlockedError{UsedTokens: 101, Limit: 100}
	}
	return nil
}

func (c *recordingContextCompactor) Compact(_ context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
	request.Messages = append([]engine.Message(nil), request.Messages...)
	request.ToolDefinitions = append([]engine.ToolDefinition(nil), request.ToolDefinitions...)
	c.requests = append(c.requests, request)
	return c.proposal, nil
}

func (*recordingContextCompactor) CheckContext(context.Context, ContextBudgetRequest) error {
	return nil
}

func ordinaryConversationRequest(value string) engine.ConversationRequest {
	return engine.ConversationRequest{
		Input: engine.RunInput{Prompt: value}, Turn: 1, Phase: engine.PhaseAction,
		Preparation: engine.ConversationPrepareNormal,
	}
}

func messageContents(messages []engine.Message) []string {
	result := make([]string, len(messages))
	for index, message := range messages {
		result[index] = message.Content
	}
	return result
}

func recordContents(records []session.MessageRecord) []string {
	result := make([]string, len(records))
	for index, record := range records {
		result[index] = record.Message.Content
	}
	return result
}

func containsAll(value string, tokens ...string) bool {
	for _, token := range tokens {
		if !strings.Contains(value, token) {
			return false
		}
	}
	return true
}

func assertSingleSummaryProjection(t *testing.T, messages []engine.Message, final string) {
	t.Helper()
	contents := messageContents(messages)
	if len(contents) < 3 || contents[0] != "system" || contents[len(contents)-1] != final {
		t.Fatalf("projection = %v", contents)
	}
	count := 0
	for _, content := range contents {
		if strings.Contains(content, "manual summary") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("summary count = %d in %v", count, contents)
	}
}
