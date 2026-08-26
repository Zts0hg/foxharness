package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestRuntimeHarnessRunAssemblesFrozenDependenciesAndLifecycle(t *testing.T) {
	store := newLifecycleStore()
	var initialized AgentSessionSnapshot
	var assembled []RunAssembly
	observer := &recordingRunObserver{}
	dependencies := successfulHarnessDependencies(&assembled)
	dependencies.InitializeSession = func(_ context.Context, snapshot AgentSessionSnapshot) error {
		initialized = snapshot
		return nil
	}

	harness, err := NewRuntimeHarness(store, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	agentSession, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if initialized != agentSession.Snapshot() {
		t.Fatalf("initialized session = %#v, want %#v", initialized, agentSession.Snapshot())
	}

	turns := 3
	allowed := []string{"read_file"}
	result, err := agentSession.Run(context.Background(), RunSpec{
		Prompt: "inspect", Model: "model-a", Effort: "high", ProviderProtocol: "messages",
		MaxTurns: &turns, AllowedTools: allowed, Observer: observer,
	})
	if err != nil {
		t.Fatal(err)
	}
	turns = 99
	allowed[0] = "write_file"
	if got := result.Outcome; got.FinalMessage != "done" || got.FinishReason != "stop" || got.TurnCount != 1 {
		t.Fatalf("outcome = %#v", got)
	}
	if result.SessionID != agentSession.Snapshot().ID || result.RunID == "" {
		t.Fatalf("result identity = %#v", result)
	}
	if len(assembled) != 4 {
		t.Fatalf("assembly calls = %d, want model/tools/policy/context", len(assembled))
	}
	for _, request := range assembled {
		if request.Session.ID != agentSession.Snapshot().ID || request.Run.Model != "model-a" || request.Run.MaxTurns != 3 {
			t.Fatalf("assembly request = %#v", request)
		}
		if !reflect.DeepEqual(request.AllowedTools, []string{"read_file"}) {
			t.Fatalf("assembly tools = %v", request.AllowedTools)
		}
	}
	if store.startCount() != 1 || store.finishCount() != 1 {
		t.Fatalf("stored lifecycle starts/finishes = %d/%d", store.startCount(), store.finishCount())
	}
	records := store.messageRecords(agentSession.Snapshot().ID)
	if len(records) != 2 || records[0].Message.Content != "inspect" || records[1].Message.Content != "done" {
		t.Fatalf("persisted messages = %#v", records)
	}
	facts := observer.snapshot()
	if got := runtimeFactKinds(facts); !reflect.DeepEqual(got, []engine.FactKind{engine.FactRunStarted, engine.FactMessage, engine.FactRunCompleted}) {
		t.Fatalf("facts = %v", got)
	}
	for _, fact := range facts {
		if fact.SessionID != agentSession.Snapshot().ID || fact.RunID == "" {
			t.Fatalf("uncorrelated fact = %#v", fact)
		}
	}
}

func TestRuntimeHarnessRunPreservesExplicitEmptyAllowedTools(t *testing.T) {
	store := newLifecycleStore()
	var assembled []RunAssembly
	harness, err := NewRuntimeHarness(store, successfulHarnessDependencies(&assembled))
	if err != nil {
		t.Fatal(err)
	}
	agentSession, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect", AllowedTools: []string{}}); err != nil {
		t.Fatal(err)
	}
	if len(assembled) == 0 {
		t.Fatal("expected runtime assembly calls")
	}
	for _, request := range assembled {
		if request.AllowedTools == nil || len(request.AllowedTools) != 0 {
			t.Fatalf("assembly tools = %#v, want explicit empty deny-all slice", request.AllowedTools)
		}
	}
}

func TestRuntimeHarnessArtifactAndTelemetryFailuresAreWarnings(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	artifact := &failingArtifactJournal{err: errors.New("transcript unavailable")}
	telemetry := &failingTelemetryJournal{err: errors.New("metrics unavailable")}
	dependencies.NewArtifactJournal = func(context.Context, RunAssembly) (SessionArtifactJournal, error) {
		return artifact, nil
	}
	dependencies.NewTelemetryJournal = func(context.Context, RunAssembly) (TelemetryJournal, error) {
		return telemetry, nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect"})
	if err != nil {
		t.Fatalf("best-effort journal failure became terminal: %v", err)
	}
	wantWarnings := []RunWarning{
		{Sink: "artifact", Operation: "record_fact", Error: "transcript unavailable"},
		{Sink: "telemetry", Operation: "record_fact", Error: "metrics unavailable"},
		{Sink: "artifact", Operation: "record_fact", Error: "transcript unavailable"},
		{Sink: "telemetry", Operation: "record_fact", Error: "metrics unavailable"},
		{Sink: "artifact", Operation: "record_fact", Error: "transcript unavailable"},
		{Sink: "telemetry", Operation: "record_fact", Error: "metrics unavailable"},
	}
	if !reflect.DeepEqual(result.Warnings, wantWarnings) {
		t.Fatalf("warnings = %#v, want %#v", result.Warnings, wantWarnings)
	}
	if artifact.calls != 3 || telemetry.calls != 3 {
		t.Fatalf("journal calls = artifact:%d telemetry:%d", artifact.calls, telemetry.calls)
	}
}

func TestRuntimeHarnessArtifactAndTelemetryPanicsAreWarnings(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	artifact := &panickingArtifactJournal{}
	telemetry := &panickingTelemetryJournal{}
	dependencies.NewArtifactJournal = func(context.Context, RunAssembly) (SessionArtifactJournal, error) {
		return artifact, nil
	}
	dependencies.NewTelemetryJournal = func(context.Context, RunAssembly) (TelemetryJournal, error) {
		return telemetry, nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect"})
	if err != nil {
		t.Fatalf("best-effort journal panic became terminal: %v", err)
	}
	if result.Outcome.FinalMessage != "done" {
		t.Fatalf("outcome = %#v, want successful authoritative result", result.Outcome)
	}
	wantWarnings := []RunWarning{
		{Sink: "artifact", Operation: "record_fact", Error: "panic: artifact panic"},
		{Sink: "telemetry", Operation: "record_fact", Error: "panic: telemetry panic"},
		{Sink: "artifact", Operation: "record_fact", Error: "panic: artifact panic"},
		{Sink: "telemetry", Operation: "record_fact", Error: "panic: telemetry panic"},
		{Sink: "artifact", Operation: "record_fact", Error: "panic: artifact panic"},
		{Sink: "telemetry", Operation: "record_fact", Error: "panic: telemetry panic"},
	}
	if !reflect.DeepEqual(result.Warnings, wantWarnings) {
		t.Fatalf("warnings = %#v, want %#v", result.Warnings, wantWarnings)
	}
	if artifact.calls != 3 || telemetry.calls != 3 {
		t.Fatalf("journal calls = artifact:%d telemetry:%d", artifact.calls, telemetry.calls)
	}
}

func TestRuntimeHarnessJournalInitializationFailuresAreWarnings(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewArtifactJournal = func(context.Context, RunAssembly) (SessionArtifactJournal, error) {
		return nil, errors.New("transcript setup failed")
	}
	dependencies.NewTelemetryJournal = func(context.Context, RunAssembly) (TelemetryJournal, error) {
		return nil, errors.New("telemetry setup failed")
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect"})
	if err != nil {
		t.Fatalf("journal initialization failure became terminal: %v", err)
	}
	want := []RunWarning{
		{Sink: "artifact", Operation: "initialize", Error: "transcript setup failed"},
		{Sink: "telemetry", Operation: "initialize", Error: "telemetry setup failed"},
	}
	if !reflect.DeepEqual(result.Warnings, want) {
		t.Fatalf("warnings = %#v, want %#v", result.Warnings, want)
	}
}

func TestRuntimeHarnessSessionInitializationFailureReleasesLiveLease(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	initializationErr := errors.New("working memory unavailable")
	dependencies.InitializeSession = func(context.Context, AgentSessionSnapshot) error {
		return initializationErr
	}
	harness, _ := NewRuntimeHarness(store, dependencies)

	if _, err := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"}); !errors.Is(err, initializationErr) {
		t.Fatalf("CreateSession() error = %v", err)
	}
	createdID := session.ID("session-1")
	opened, err := harness.OpenSession(context.Background(), CLIExec, createdID)
	if err != nil {
		t.Fatalf("initialization failure retained a live lease: %v", err)
	}
	if err := opened.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeHarnessAuthoritativeMessageFailureIsTerminalAndFinishesRun(t *testing.T) {
	store := newLifecycleStore()
	store.failNextMessage(errors.New("message store unavailable"))
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect"})
	if err == nil || result.Outcome.ErrorKind != "conversation" || !errors.Is(result.Outcome.Err, err) {
		t.Fatalf("Run() = %#v, %v; want fatal conversation persistence error", result, err)
	}
	if store.finishCount() != 1 {
		t.Fatalf("finish count = %d, want 1", store.finishCount())
	}
}

func TestRuntimeHarnessTerminalObserverUsesFreshBoundedContext(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	agentSession, _ := harness.CreateSession(context.Background(), TUIInteractive, SessionOptions{WorkDir: "/workspace"})
	observer := &terminalContextObserver{}

	if _, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect", Observer: observer}); err != nil {
		t.Fatal(err)
	}
	if !observer.terminalDeadline {
		t.Fatal("terminal observer context has no bounded deadline")
	}
}

func TestRuntimeHarnessEnforcesProfileCapacityAcrossDistinctSessions(t *testing.T) {
	store := newLifecycleStore()
	started := make(chan struct{}, 5)
	release := make(chan struct{})
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(_ context.Context, request RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(ctx context.Context, _ engine.RunContext) (engine.ModelResult, error) {
			started <- struct{}{}
			select {
			case <-ctx.Done():
				return engine.ModelResult{}, ctx.Err()
			case <-release:
				return completedModelResult(), nil
			}
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	sessions := make([]*AgentSession, 5)
	for index := range sessions {
		sessions[index], _ = harness.CreateSession(context.Background(), FeishuRemote, SessionOptions{WorkDir: "/workspace"})
	}

	errs := make(chan error, len(sessions)-1)
	for _, agentSession := range sessions[:4] {
		go func(agentSession *AgentSession) {
			_, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect"})
			errs <- err
		}(agentSession)
	}
	for range 4 {
		<-started
	}
	blockedContext := newObservedCancelContext()
	blockedResult := make(chan error, 1)
	go func() {
		_, err := sessions[4].Run(blockedContext, RunSpec{Prompt: "inspect"})
		blockedResult <- err
	}()
	<-blockedContext.doneObserved
	<-blockedContext.doneObserved
	blockedContext.cancel()
	if err := <-blockedResult; !errors.Is(err, context.Canceled) {
		t.Fatalf("fifth run error = %v, want capacity wait cancellation", err)
	}
	if got := store.startCount(); got != 4 {
		t.Fatalf("stored runs admitted = %d, want profile capacity 4", got)
	}
	close(release)
	for range sessions[:4] {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
}

func TestRuntimeHarnessReturnsArtifactPathsFromCanonicalToolFacts(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	invocations := 0
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(context.Context, engine.RunContext) (engine.ModelResult, error) {
			invocations++
			if invocations == 1 {
				return engine.ModelResult{Message: schema.Message{
					Role:      engine.RoleAssistant,
					ToolCalls: []schema.ToolCall{{ID: "call-1", Name: "inspect"}},
				}, FinishReason: "tool_calls"}, nil
			}
			return completedModelResult(), nil
		}), nil
	}
	dependencies.NewTools = func(context.Context, RunAssembly) (engine.ToolExecutor, error) {
		return artifactToolExecutor{}, nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.ArtifactPaths, []string{"/artifacts/call-1.txt"}) {
		t.Fatalf("artifact paths = %v", result.ArtifactPaths)
	}
	if got := store.messageRecords(agentSession.Snapshot().ID); len(got) != 4 || got[2].Message.ToolCallID != "call-1" {
		t.Fatalf("persisted tool conversation = %#v", got)
	}
}

func TestRuntimeHarnessFactoryFailureEmitsCorrelatedTerminalFactsAndFinishes(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return nil, errors.New("provider selection failed")
	}
	observer := &recordingRunObserver{}
	harness, _ := NewRuntimeHarness(store, dependencies)
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect", Observer: observer})
	if err == nil || result.Outcome.ErrorKind != "provider" {
		t.Fatalf("Run() = %#v, %v; want provider assembly failure", result, err)
	}
	facts := observer.snapshot()
	if got := runtimeFactKinds(facts); !reflect.DeepEqual(got, []engine.FactKind{engine.FactRunStarted, engine.FactRunError}) {
		t.Fatalf("facts = %v", got)
	}
	if facts[0].Fact.Sequence != 1 || facts[1].Fact.Sequence != 2 ||
		facts[0].SessionID != agentSession.Snapshot().ID || facts[0].RunID != result.RunID {
		t.Fatalf("factory failure facts = %#v", facts)
	}
	if store.finishCount() != 1 {
		t.Fatalf("finish count = %d, want 1", store.finishCount())
	}
}

func TestRuntimeHarnessTreatsTypedNilObserverAsAbsent(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	var observer RunObserverFunc

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect", Observer: observer})
	if err != nil || result.Outcome.FinalMessage != "done" {
		t.Fatalf("Run() = %#v, %v", result, err)
	}
}

func TestRuntimeHarnessContainsRunObserverPanicsAsWarnings(t *testing.T) {
	store := newLifecycleStore()
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})

	result, err := agentSession.Run(context.Background(), RunSpec{
		Prompt:   "inspect",
		Observer: RunObserverFunc(func(context.Context, RuntimeFact) { panic("presentation unavailable") }),
	})
	if err != nil || result.Outcome.FinalMessage != "done" {
		t.Fatalf("Run() = %#v, %v; want successful runtime outcome", result, err)
	}
	if store.finishCount() != 1 {
		t.Fatalf("finish count = %d, want 1", store.finishCount())
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("warnings = %#v, want one warning per observer panic", result.Warnings)
	}
	for _, warning := range result.Warnings {
		if warning.Sink != "observer" || warning.Operation != "observe_fact" ||
			!strings.Contains(warning.Error, "presentation unavailable") {
			t.Fatalf("observer warning = %#v", warning)
		}
	}
}

func TestRuntimeHarnessAppliesResolvedTaskTimeout(t *testing.T) {
	store := newLifecycleStore()
	dependencies := successfulHarnessDependencies(nil)
	dependencies.NewModel = func(context.Context, RunAssembly) (engine.ModelInvoker, error) {
		return runtimeModelInvokerFunc(func(ctx context.Context, _ engine.RunContext) (engine.ModelResult, error) {
			<-ctx.Done()
			return engine.ModelResult{}, ctx.Err()
		}), nil
	}
	harness, _ := NewRuntimeHarness(store, dependencies)
	agentSession, _ := harness.CreateSession(context.Background(), BenchmarkEval, SessionOptions{WorkDir: "/workspace"})
	timeout := time.Millisecond

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect", TaskTimeout: &timeout})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) || result.Outcome.ErrorKind != "provider" {
		t.Fatalf("Run() = %#v, %v; want deadline failure", result, err)
	}
	if store.finishCount() != 1 {
		t.Fatalf("finish count = %d, want 1", store.finishCount())
	}
}

func TestRuntimeHarnessFinishFailureCanBeRecoveredWithoutLeakingScope(t *testing.T) {
	store := newLifecycleStore()
	store.failNextFinish(errors.New("run metadata unavailable"))
	harness, _ := NewRuntimeHarness(store, successfulHarnessDependencies(nil))
	agentSession, _ := harness.CreateSession(context.Background(), CLIExec, SessionOptions{WorkDir: "/workspace"})
	observer := &recordingRunObserver{}

	result, err := agentSession.Run(context.Background(), RunSpec{Prompt: "inspect", Observer: observer})
	if err == nil || result.Outcome.ErrorKind != "persistence" {
		t.Fatalf("Run() = %#v, %v; want terminal persistence failure", result, err)
	}
	if got := runtimeFactKinds(observer.snapshot()); !reflect.DeepEqual(got, []engine.FactKind{
		engine.FactRunStarted, engine.FactMessage, engine.FactRunError,
	}) {
		t.Fatalf("finish failure facts = %v, want one error terminal", got)
	}
	if err := agentSession.RecoverRunFinish(context.Background()); err != nil {
		t.Fatalf("RecoverRunFinish() error = %v", err)
	}
	if _, err := agentSession.Run(context.Background(), RunSpec{Prompt: "next"}); err != nil {
		t.Fatalf("run after recovery = %v", err)
	}
}

func successfulHarnessDependencies(assembled *[]RunAssembly) HarnessDependencies {
	record := func(request RunAssembly) {
		if assembled != nil {
			*assembled = append(*assembled, request)
		}
	}
	return HarnessDependencies{
		NewModel: func(_ context.Context, request RunAssembly) (engine.ModelInvoker, error) {
			record(request)
			return runtimeModelInvokerFunc(func(context.Context, engine.RunContext) (engine.ModelResult, error) {
				return completedModelResult(), nil
			}), nil
		},
		NewTools: func(_ context.Context, request RunAssembly) (engine.ToolExecutor, error) {
			record(request)
			return runtimeToolExecutor{}, nil
		},
		NewPolicy: func(_ context.Context, request RunAssembly) (engine.TurnPolicy, error) {
			record(request)
			return runtimeTurnPolicy{}, nil
		},
		NewContext: func(_ context.Context, request RunAssembly) (ContextCollector, ContextCompactor, error) {
			record(request)
			return runtimeContextCollector{}, nil, nil
		},
	}
}

type runtimeModelInvokerFunc func(context.Context, engine.RunContext) (engine.ModelResult, error)

func (f runtimeModelInvokerFunc) StartRun(context.Context) (engine.ModelRunInvoker, error) {
	return runtimeModelRunInvokerFunc(f), nil
}

type runtimeModelRunInvokerFunc func(context.Context, engine.RunContext) (engine.ModelResult, error)

func (f runtimeModelRunInvokerFunc) Invoke(ctx context.Context, request engine.RunContext, _ engine.ModelFactEmitter) (engine.ModelResult, error) {
	return f(ctx, request)
}

func completedModelResult() engine.ModelResult {
	return engine.ModelResult{
		Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}, FinishReason: "stop",
	}
}

type runtimeToolSnapshot struct{}

func (runtimeToolSnapshot) ToolDefinitions() []engine.ToolDefinition { return nil }

type runtimeToolExecutor struct{}

func (runtimeToolExecutor) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	return runtimeToolSnapshot{}, nil
}

func (runtimeToolExecutor) Execute(context.Context, engine.ToolSnapshot, []engine.ToolCall) (engine.ToolBatch, error) {
	return engine.ToolBatch{}, nil
}

type artifactToolExecutor struct{}

func (artifactToolExecutor) Snapshot(context.Context) (engine.ToolSnapshot, error) {
	return runtimeToolSnapshot{}, nil
}

func (artifactToolExecutor) Execute(_ context.Context, _ engine.ToolSnapshot, calls []engine.ToolCall) (engine.ToolBatch, error) {
	return engine.ToolBatch{Results: []engine.ToolExecutionResult{{
		CallID: calls[0].ID, FullContent: "full", ModelContent: "preview",
		ObserverContent: "preview", ArtifactPath: "/artifacts/call-1.txt",
	}}}, nil
}

type runtimeTurnPolicy struct{}

func (runtimeTurnPolicy) StartRun(context.Context, engine.RunInput) (engine.TurnRunPolicy, error) {
	return runtimeTurnRunPolicy{}, nil
}

type runtimeTurnRunPolicy struct{}

func (runtimeTurnRunPolicy) BeforeTurn(context.Context, engine.TurnState) (engine.PolicyChanges, error) {
	return engine.PolicyChanges{}, nil
}

func (runtimeTurnRunPolicy) AfterModel(context.Context, engine.TurnState) (engine.TurnDecision, error) {
	return engine.TurnDecision{Complete: true}, nil
}

func (runtimeTurnRunPolicy) AfterTools(context.Context, engine.ToolState) (engine.PolicyChanges, error) {
	return engine.PolicyChanges{}, nil
}

type runtimeContextCollector struct{}

func (runtimeContextCollector) Collect(context.Context, ContextCollectionRequest) ([]prompt.Fragment, error) {
	return []prompt.Fragment{prompt.Text("system")}, nil
}

type recordingRunObserver struct {
	mu    sync.Mutex
	facts []RuntimeFact
}

type terminalContextObserver struct {
	terminalDeadline bool
}

func (o *terminalContextObserver) ObserveRunFact(ctx context.Context, fact RuntimeFact) {
	if fact.Fact.Kind != engine.FactRunCompleted && fact.Fact.Kind != engine.FactRunError {
		return
	}
	_, o.terminalDeadline = ctx.Deadline()
}

func (o *recordingRunObserver) ObserveRunFact(_ context.Context, fact RuntimeFact) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.facts = append(o.facts, fact)
}

func (o *recordingRunObserver) snapshot() []RuntimeFact {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]RuntimeFact(nil), o.facts...)
}

type failingArtifactJournal struct {
	calls int
	err   error
}

func (j *failingArtifactJournal) RecordArtifact(context.Context, RuntimeFact) error {
	j.calls++
	return j.err
}

type failingTelemetryJournal struct {
	calls int
	err   error
}

func (j *failingTelemetryJournal) RecordTelemetry(context.Context, RuntimeFact) error {
	j.calls++
	return j.err
}

type panickingArtifactJournal struct{ calls int }

func (j *panickingArtifactJournal) RecordArtifact(context.Context, RuntimeFact) error {
	j.calls++
	panic("artifact panic")
}

type panickingTelemetryJournal struct{ calls int }

func (j *panickingTelemetryJournal) RecordTelemetry(context.Context, RuntimeFact) error {
	j.calls++
	panic("telemetry panic")
}

func runtimeFactKinds(facts []RuntimeFact) []engine.FactKind {
	result := make([]engine.FactKind, len(facts))
	for index, fact := range facts {
		result[index] = fact.Fact.Kind
	}
	return result
}

type observedCancelContext struct {
	done         chan struct{}
	doneObserved chan struct{}
	once         sync.Once
}

func newObservedCancelContext() *observedCancelContext {
	return &observedCancelContext{done: make(chan struct{}), doneObserved: make(chan struct{}, 4)}
}

func (*observedCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *observedCancelContext) Done() <-chan struct{} {
	c.doneObserved <- struct{}{}
	return c.done
}

func (c *observedCancelContext) Err() error {
	select {
	case <-c.done:
		return context.Canceled
	default:
		return nil
	}
}

func (*observedCancelContext) Value(any) any { return nil }

func (c *observedCancelContext) cancel() {
	c.once.Do(func() { close(c.done) })
}
