package runtime

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestContextControllerRejectsTypedNilCollector(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	var collector *recordingContextCollector

	if _, err := agentSession.NewContextController(scope, collector, nil); err == nil {
		t.Fatal("NewContextController() error = nil, want typed-nil collector rejection")
	}
	_ = agentSession.FinishRun(scope)
}

func TestContextControllerTreatsTypedNilCompactorAsAbsent(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	var compactor *budgetCheckingCompactor
	controller, err := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if err != nil {
		t.Fatalf("NewContextController() error = %v, want typed-nil optional compactor treated as absent", err)
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Prepare() panicked for typed-nil optional compactor: %v", recovered)
		}
	}()
	if _, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work")); err != nil {
		t.Fatalf("Prepare() error = %v, want typed-nil optional compactor treated as absent", err)
	}
	_ = agentSession.FinishRun(scope)
}

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
	/* Turn 1 keeps both baseline decision points; every later turn compacts at
	 * most once, on its first prepare. */
	want := []ContextCompactionTrigger{
		ContextCompactionInitialHistory, ContextCompactionPreTurn, ContextCompactionPreTurn,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compaction triggers = %v, want %v", got, want)
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerTurnOneKeepsBothCompactionDecisionPoints pins the
 * baseline turn shape: the first prepare of a run compacts the durable
 * session history and the second prepare of the same turn still runs the
 * ordinary run-local pre-turn decision. */
func TestContextControllerTurnOneKeepsBothCompactionDecisionPoints(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)

	thinking := ordinaryConversationRequest("work")
	thinking.Phase = engine.PhaseThinking
	if _, err := controller.Prepare(context.Background(), thinking); err != nil {
		t.Fatal(err)
	}
	action := ordinaryConversationRequest("work")
	if _, err := controller.Prepare(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	got := compactionTriggers(compactor)
	want := []ContextCompactionTrigger{ContextCompactionInitialHistory, ContextCompactionPreTurn}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("turn 1 compaction triggers = %v, want %v", got, want)
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerChecksBudgetOnFirstTurnAfterDurableCompaction pins the
 * baseline blocking decision on a first turn without thinking: the durable
 * session-history compaction never suppresses the budget check. */
func TestContextControllerChecksBudgetOnFirstTurnAfterDurableCompaction(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	id := agentSession.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, Message: engine.Message{Role: engine.RoleUser, Content: "covered"}},
	})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := contextCompactorFunc(func(_ context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
		if request.Trigger == ContextCompactionInitialHistory {
			return ContextCompactionProposal{
				Changed:      true,
				CompactState: &session.CompactState{Summary: "durable summary", CoveredUntilSeq: 0},
			}, nil
		}
		return ContextCompactionProposal{}, nil
	})
	budget := &blockedBudgetCompactor{inner: compactor}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), budget)
	_, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
	var blocked *ContextBlockedError
	if !errors.As(err, &blocked) {
		t.Fatalf("first-turn Prepare() error = %v, want the blocking decision after the durable compaction", err)
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerSkipsActionBudgetAfterRunLocalCompaction pins the
 * baseline blocking decision on a later thinking turn: a run-local compaction
 * performed during the same turn suppresses the action-phase budget check. */
func TestContextControllerSkipsActionBudgetAfterRunLocalCompaction(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	id := agentSession.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, Message: engine.Message{Role: engine.RoleUser, Content: "covered"}},
	})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := contextCompactorFunc(func(_ context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
		if request.Trigger == ContextCompactionInitialHistory {
			return ContextCompactionProposal{}, nil
		}
		if request.Trigger == ContextCompactionPreTurn {
			return ContextCompactionProposal{Changed: true, Messages: []engine.Message{
				{Role: engine.RoleSystem, Content: "system"},
				{Role: engine.RoleUser, Content: "compacted"},
			}}, nil
		}
		return ContextCompactionProposal{}, nil
	})
	budget := &blockedBudgetCompactor{inner: compactor}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), budget)
	thinking := ordinaryConversationRequest("work")
	thinking.Turn = 2
	thinking.Phase = engine.PhaseThinking
	if _, err := controller.Prepare(context.Background(), thinking); err != nil {
		t.Fatal(err)
	}
	action := ordinaryConversationRequest("work")
	action.Turn = 2
	if _, err := controller.Prepare(context.Background(), action); err != nil {
		t.Fatalf("action Prepare() after a run-local compaction = %v, want the baseline continuation", err)
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerCompactsBeforeTurnInjections pins the baseline per-turn
 * order: the pre-turn compaction runs on the projection without the turn's
 * injected recovery and reminder notices, while the returned projection still
 * carries them for the model. */
func TestContextControllerCompactsBeforeTurnInjections(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
	compactor := &recordingContextCompactor{}
	controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
	if _, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work")); err != nil {
		t.Fatal(err)
	}
	if err := controller.RequestChanges(context.Background(), []engine.ConversationChange{{
		Kind: engine.ConversationAppendContextMessage, Source: engine.ConversationSourceReminder,
		Message: engine.Message{Role: engine.RoleUser, Content: "queued reminder"},
	}}); err != nil {
		t.Fatal(err)
	}
	request := ordinaryConversationRequest("work")
	request.Turn = 2
	projection, err := controller.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) < 2 {
		t.Fatalf("compaction requests = %#v", compactor.requests)
	}
	for _, content := range messageContents(compactor.requests[1].Messages) {
		if strings.Contains(content, "queued reminder") {
			t.Fatalf("turn notice reached the compaction input: %#v", compactor.requests[1].Messages)
		}
	}
	if got := messageContents(projection.Context.Messages); !slices.Contains(got, "queued reminder") {
		t.Fatalf("projection lost the turn notice: %v", got)
	}
	_ = agentSession.FinishRun(scope)
}

/* TestContextControllerKeepsBaselinePersistenceErrorChains ports the baseline
 * lifecycle contract: run-facing persistence and context-assembly failures
 * keep the baseline wrapped error wording. */
func TestContextControllerKeepsBaselinePersistenceErrorChains(t *testing.T) {
	tests := []struct {
		name      string
		kind      string
		wantError string
	}{
		{name: "user message persistence", kind: "user", wantError: "写入 Session 用户消息失败: message log unavailable"},
		{name: "assistant message persistence", kind: "assistant", wantError: "写入 Session 助手消息失败: message log unavailable"},
		{name: "tool result persistence", kind: "tool", wantError: "写入 Session 工具结果失败: message log unavailable"},
		{name: "message history load", kind: "load", wantError: "读取 Session 消息历史失败: message log unavailable"},
		{name: "initial context assembly", kind: "initial", wantError: "组装 Session 上下文失败: summarizer unavailable"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			store := newLifecycleStore()
			harness, _ := NewRuntimeHarness(store)
			agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
			scope, _ := agentSession.BeginRun(context.Background(), RunSpec{Prompt: "work"})
			var compactor ContextCompactor
			if testCase.kind == "initial" {
				compactor = failingInitialCompactor{err: errors.New("summarizer unavailable")}
			}
			controller, _ := agentSession.NewContextController(scope, staticContextCollector("system"), compactor)
			/* Both assembly failures surface on the run's first prepare, while
			 * the persistence failures surface on a later committed change. */
			if testCase.kind == "initial" || testCase.kind == "load" {
				if testCase.kind == "load" {
					store.failNextLoad(errors.New("message log unavailable"))
				}
				_, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work"))
				if err == nil || err.Error() != testCase.wantError {
					t.Fatalf("Prepare() error = %v, want %q", err, testCase.wantError)
				}
				_ = agentSession.FinishRun(scope)
				return
			}
			if _, err := controller.Prepare(context.Background(), ordinaryConversationRequest("work")); err != nil {
				t.Fatal(err)
			}
			store.failNextMessage(errors.New("message log unavailable"))
			message := engine.Message{Role: engine.RoleUser, Content: "follow-up"}
			switch testCase.kind {
			case "assistant":
				message = engine.Message{Role: engine.RoleAssistant, Content: "answer"}
			case "tool":
				message = engine.Message{Role: engine.RoleUser, Content: "tool output", ToolCallID: "call-1"}
			}
			err := controller.RequestChanges(context.Background(), []engine.ConversationChange{{
				Kind: engine.ConversationAppendMessage, Message: message,
			}})
			if err == nil || err.Error() != testCase.wantError {
				t.Fatalf("error = %v, want %q", err, testCase.wantError)
			}
			_ = agentSession.FinishRun(scope)
		})
	}
}

/* failingInitialCompactor fails only the durable session-history compaction,
 * mirroring a summarizer outage during initial context assembly. */
type failingInitialCompactor struct {
	err error
}

func (c failingInitialCompactor) Compact(_ context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
	if request.Trigger == ContextCompactionInitialHistory {
		return ContextCompactionProposal{}, c.err
	}
	return ContextCompactionProposal{}, nil
}

func (failingInitialCompactor) CheckContext(context.Context, ContextBudgetRequest) error {
	return nil
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
	if len(compactor.requests) != 3 || len(compactor.requests[2].Records) != 1 {
		t.Fatalf("compactor requests = %#v", compactor.requests)
	}
	if got := compactor.requests[2].Records[0].Time; !got.Equal(fixed) {
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
	compactor := &recordingContextCompactor{proposalFunc: func(request ContextCompactionRequest) (ContextCompactionProposal, error) {
		if request.Trigger == ContextCompactionPreTurn {
			return ContextCompactionProposal{Changed: true, Messages: []engine.Message{
				{Role: engine.RoleSystem, Content: "system"},
				{Role: engine.RoleUser, Content: "summary"},
			}}, nil
		}
		return ContextCompactionProposal{
			Changed:      true,
			CompactState: &session.CompactState{Summary: "summary", CoveredUntilSeq: 0},
		}, nil
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
	if len(compactor.requests) != 2 || compactor.requests[0].Trigger != ContextCompactionInitialHistory || compactor.requests[0].ToolDefinitions[0].Name != "read_file" {
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

func TestAgentSessionRejectsTypedNilManualCompactor(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	var compactor *recordingContextCompactor
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("CompactContext() panicked for typed-nil compactor: %v", recovered)
		}
	}()
	if _, err := agentSession.CompactContext(context.Background(), compactor, "focus"); err == nil || !strings.Contains(err.Error(), "runtime context compactor is required") {
		t.Fatalf("CompactContext() error = %v, want typed-nil compactor rejection", err)
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

/* TestAgentSessionManualCompactionSummarizesRawStoredSummary pins the baseline
 * manual compaction input: an existing durable summary reaches the summarizer
 * as its raw text, never wrapped in the projected summary message. */
func TestAgentSessionManualCompactionSummarizesRawStoredSummary(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store)
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	id := agentSession.Snapshot().ID
	store.seedMessages(id, []session.MessageRecord{
		{Seq: 0, RunID: "old", Message: engine.Message{Role: engine.RoleUser, Content: "covered"}},
		{Seq: 1, RunID: "old", Message: engine.Message{Role: engine.RoleAssistant, Content: "active"}},
	})
	store.seedCompactState(id, &session.CompactState{Summary: "earlier summary", CoveredUntilSeq: 0})
	compactor := &recordingContextCompactor{proposal: ContextCompactionProposal{
		Changed: true, CompactState: &session.CompactState{Summary: "manual summary", CoveredUntilSeq: 1},
	}}
	if _, err := agentSession.CompactContext(context.Background(), compactor, ""); err != nil {
		t.Fatal(err)
	}
	if len(compactor.requests) != 1 {
		t.Fatalf("compaction requests = %d, want 1", len(compactor.requests))
	}
	contents := messageContents(compactor.requests[0].Messages)
	if len(contents) != 2 || contents[0] != "earlier summary" || contents[1] != "active" {
		t.Fatalf("manual compaction input = %#v, want the raw summary followed by the active records", contents)
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
	proposal     ContextCompactionProposal
	proposalFunc func(ContextCompactionRequest) (ContextCompactionProposal, error)
	requests     []ContextCompactionRequest
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
	if c.proposalFunc != nil {
		return c.proposalFunc(request)
	}
	return c.proposal, nil
}

/* blockedBudgetCompactor always refuses the budget check while delegating the
 * compaction proposal to an inner compactor. */
type blockedBudgetCompactor struct {
	inner ContextCompactor
}

func (c *blockedBudgetCompactor) Compact(ctx context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
	return c.inner.Compact(ctx, request)
}

func (*blockedBudgetCompactor) CheckContext(context.Context, ContextBudgetRequest) error {
	return &ContextBlockedError{UsedTokens: 101, Limit: 100}
}

func compactionTriggers(compactor *recordingContextCompactor) []ContextCompactionTrigger {
	triggers := make([]ContextCompactionTrigger, 0, len(compactor.requests))
	for _, request := range compactor.requests {
		triggers = append(triggers, request.Trigger)
	}
	return triggers
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

/* TestContextBlockedErrorKeepsBaselineMessage pins the baseline blocking
 * message so surfaced run errors keep the baseline threshold wording. */
func TestContextBlockedErrorKeepsBaselineMessage(t *testing.T) {
	blocked := &ContextBlockedError{UsedTokens: 130000, Limit: 120000}
	if got, want := blocked.Error(), "上下文 token 数 (130000) 超过阻塞阈值 (120000)，无法继续发送请求"; got != want {
		t.Fatalf("ContextBlockedError.Error() = %q, want %q", got, want)
	}
}
