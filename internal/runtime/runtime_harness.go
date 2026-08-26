package runtime

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/session"
)

const terminalObserverTimeout = 5 * time.Second

/* RunAssembly is the immutable identity and policy snapshot supplied to one run factory. */
type RunAssembly struct {
	Session      AgentSessionSnapshot
	Run          RunScopeSnapshot
	Spec         RunSnapshot
	AllowedTools []string
	Permission   PermissionScope
	ChildRunner  *ChildRunner
}

/* RuntimeFact correlates one canonical engine fact with its owning session and run. */
type RuntimeFact struct {
	SessionID session.ID
	RunID     session.RunID
	Fact      engine.Fact
}

/* RunObserver synchronously receives correlated user-observable runtime facts. */
type RunObserver interface {
	ObserveRunFact(context.Context, RuntimeFact)
}

/* RunObserverFunc adapts a function to RunObserver. */
type RunObserverFunc func(context.Context, RuntimeFact)

/* ObserveRunFact invokes the adapted observer function. */
func (f RunObserverFunc) ObserveRunFact(ctx context.Context, fact RuntimeFact) {
	f(ctx, fact)
}

/* SessionArtifactJournal records non-authoritative session artifacts from canonical facts. */
type SessionArtifactJournal interface {
	RecordArtifact(context.Context, RuntimeFact) error
}

/* TelemetryJournal records best-effort metrics or tracing data from canonical facts. */
type TelemetryJournal interface {
	RecordTelemetry(context.Context, RuntimeFact) error
}

/* RunWarning reports a non-terminal failure from one best-effort runtime sink. */
type RunWarning struct {
	Sink      string
	Operation string
	Error     string
}

/* RunResult combines the engine outcome with runtime-owned artifacts and warnings. */
type RunResult struct {
	SessionID        session.ID
	RunID            session.RunID
	Outcome          engine.RunOutcome
	ArtifactPaths    []string
	Warnings         []RunWarning
	CommittedMessage string
}

/* HarnessDependencies contains immutable factories and shared dependency hooks. */
type HarnessDependencies struct {
	InitializeSession   func(context.Context, AgentSessionSnapshot) error
	NewModel            func(context.Context, RunAssembly) (engine.ModelInvoker, error)
	NewTools            func(context.Context, RunAssembly) (engine.ToolExecutor, error)
	NewPolicy           func(context.Context, RunAssembly) (engine.TurnPolicy, error)
	NewContext          func(context.Context, RunAssembly) (ContextCollector, ContextCompactor, error)
	NewArtifactJournal  func(context.Context, RunAssembly) (SessionArtifactJournal, error)
	NewTelemetryJournal func(context.Context, RunAssembly) (TelemetryJournal, error)
}

/* Run executes one fully assembled target-engine run and always completes durable run lifecycle. */
func (s *AgentSession) Run(ctx context.Context, spec RunSpec) (result RunResult, runErr error) {
	scope, err := s.BeginRun(ctx, spec)
	if err != nil {
		return RunResult{}, err
	}
	assembly := RunAssembly{
		Session: s.Snapshot(), Run: scope.Snapshot(), Spec: scope.resolved.Snapshot(),
		AllowedTools: scope.AllowedTools(), Permission: scope.Permission(),
	}
	observer := newRuntimeObserver(assembly, scope.Observer())
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("runtime run panic: %v", recovered)
			result.Outcome, runErr = failedRuntimeOutcome(scope.Context(), observer, "panic", panicErr)
			result, runErr = s.finishRun(scope, observer, result, runErr)
		}
	}()
	assembly.ChildRunner, err = s.NewChildRunner(scope)
	if err != nil {
		result.Outcome, runErr = failedAssemblyOutcome(scope.Context(), observer, "child", err)
		return s.finishRun(scope, observer, result, runErr)
	}
	permission, err := resolveRunPermission(scope.Context(), scope, assembly)
	if err != nil {
		result.Outcome, runErr = failedAssemblyOutcome(scope.Context(), observer, "permission", err)
		return s.finishRun(scope, observer, result, runErr)
	}
	assembly.Permission = permission

	artifactJournal, warning := buildArtifactJournal(scope.Context(), s.dependencies, assembly)
	if warning != nil {
		observer.addWarning(*warning)
	}
	telemetryJournal, warning := buildTelemetryJournal(scope.Context(), s.dependencies, assembly)
	if warning != nil {
		observer.addWarning(*warning)
	}
	observer.artifacts = artifactJournal
	observer.telemetry = telemetryJournal

	model, err := buildModel(scope.Context(), s.dependencies, assembly)
	if err != nil {
		result.Outcome, runErr = failedAssemblyOutcome(scope.Context(), observer, "provider", err)
	} else {
		result.Outcome, runErr = s.runAssembled(scope, assembly, observer, model)
	}

	return s.finishRun(scope, observer, result, runErr)
}

func (s *AgentSession) finishRun(scope *RunScope, observer *runtimeObserver, result RunResult, runErr error) (RunResult, error) {
	finishErr := s.FinishRun(scope)
	if finishErr != nil {
		if runErr == nil {
			result.Outcome.ErrorKind = "persistence"
			result.Outcome.Err = finishErr
		}
		runErr = errors.Join(runErr, fmt.Errorf("finish runtime run: %w", finishErr))
	}
	terminalCtx, cancel := context.WithTimeout(context.WithoutCancel(scope.Context()), terminalObserverTimeout)
	observer.finish(terminalCtx, runErr, finishErr != nil)
	cancel()
	result = observer.result(result.Outcome)
	return result, runErr
}

func (s *AgentSession) runAssembled(
	scope *RunScope,
	assembly RunAssembly,
	observer *runtimeObserver,
	model engine.ModelInvoker,
) (engine.RunOutcome, error) {
	tools, err := buildTools(scope.Context(), s.dependencies, assembly)
	if err != nil {
		return failedAssemblyOutcome(scope.Context(), observer, "tool", err)
	}
	policy, err := buildPolicy(scope.Context(), s.dependencies, assembly)
	if err != nil {
		return failedAssemblyOutcome(scope.Context(), observer, "policy", err)
	}
	collector, compactor, err := buildContext(scope.Context(), s.dependencies, assembly)
	if err != nil {
		return failedAssemblyOutcome(scope.Context(), observer, "conversation", err)
	}
	conversation, err := s.NewContextController(scope, collector, compactor)
	if err != nil {
		return failedAssemblyOutcome(scope.Context(), observer, "conversation", err)
	}

	input := engine.RunInput{
		Prompt: assembly.Spec.Prompt, MaxTurns: assembly.Spec.MaxTurns, Thinking: assembly.Spec.Thinking,
	}
	return engine.NewAgentEngine(model, tools, conversation, policy, observer).Run(scope.Context(), input)
}

func failedAssemblyOutcome(ctx context.Context, observer *runtimeObserver, kind string, err error) (engine.RunOutcome, error) {
	wrapped := fmt.Errorf("assemble runtime %s: %w", kind, err)
	return failedRuntimeOutcome(ctx, observer, kind, wrapped)
}

func failedRuntimeOutcome(ctx context.Context, observer *runtimeObserver, kind string, err error) (engine.RunOutcome, error) {
	observer.observeFailure(ctx, err)
	return engine.RunOutcome{ErrorKind: kind, Err: err}, err
}

func buildModel(ctx context.Context, dependencies HarnessDependencies, request RunAssembly) (engine.ModelInvoker, error) {
	if dependencies.NewModel == nil {
		return nil, errors.New("runtime model factory is required")
	}
	value, err := dependencies.NewModel(ctx, cloneRunAssembly(request))
	if err != nil {
		return nil, err
	}
	if isNilRuntimeDependency(value) {
		return nil, errors.New("runtime model factory returned nil")
	}
	return value, nil
}

func buildTools(ctx context.Context, dependencies HarnessDependencies, request RunAssembly) (engine.ToolExecutor, error) {
	if dependencies.NewTools == nil {
		return nil, errors.New("runtime tool factory is required")
	}
	value, err := dependencies.NewTools(ctx, cloneRunAssembly(request))
	if err != nil {
		return nil, err
	}
	if isNilRuntimeDependency(value) {
		return nil, errors.New("runtime tool factory returned nil")
	}
	return value, nil
}

func buildPolicy(ctx context.Context, dependencies HarnessDependencies, request RunAssembly) (engine.TurnPolicy, error) {
	if dependencies.NewPolicy == nil {
		return nil, errors.New("runtime policy factory is required")
	}
	value, err := dependencies.NewPolicy(ctx, cloneRunAssembly(request))
	if err != nil {
		return nil, err
	}
	if isNilRuntimeDependency(value) {
		return nil, errors.New("runtime policy factory returned nil")
	}
	return value, nil
}

func buildContext(ctx context.Context, dependencies HarnessDependencies, request RunAssembly) (ContextCollector, ContextCompactor, error) {
	if dependencies.NewContext == nil {
		return nil, nil, errors.New("runtime context factory is required")
	}
	collector, compactor, err := dependencies.NewContext(ctx, cloneRunAssembly(request))
	if err != nil {
		return nil, nil, err
	}
	if isNilRuntimeDependency(collector) {
		return nil, nil, errors.New("runtime context factory returned nil collector")
	}
	if isNilRuntimeDependency(compactor) {
		compactor = nil
	}
	return collector, compactor, nil
}

func buildArtifactJournal(ctx context.Context, dependencies HarnessDependencies, request RunAssembly) (SessionArtifactJournal, *RunWarning) {
	if dependencies.NewArtifactJournal == nil {
		return nil, nil
	}
	journal, err := dependencies.NewArtifactJournal(ctx, cloneRunAssembly(request))
	if err != nil {
		return nil, &RunWarning{Sink: "artifact", Operation: "initialize", Error: err.Error()}
	}
	if isNilRuntimeDependency(journal) {
		return nil, nil
	}
	return journal, nil
}

func buildTelemetryJournal(ctx context.Context, dependencies HarnessDependencies, request RunAssembly) (TelemetryJournal, *RunWarning) {
	if dependencies.NewTelemetryJournal == nil {
		return nil, nil
	}
	journal, err := dependencies.NewTelemetryJournal(ctx, cloneRunAssembly(request))
	if err != nil {
		return nil, &RunWarning{Sink: "telemetry", Operation: "initialize", Error: err.Error()}
	}
	if isNilRuntimeDependency(journal) {
		return nil, nil
	}
	return journal, nil
}

type runtimeObserver struct {
	mu               sync.Mutex
	sessionID        session.ID
	runID            session.RunID
	observer         RunObserver
	artifacts        SessionArtifactJournal
	telemetry        TelemetryJournal
	warnings         []RunWarning
	artifactPaths    []string
	committedMessage string
	terminal         *engine.Fact
	lastSequence     int
}

func newRuntimeObserver(request RunAssembly, observer RunObserver) *runtimeObserver {
	if isNilRuntimeDependency(observer) {
		observer = nil
	}
	return &runtimeObserver{
		sessionID: request.Session.ID, runID: request.Run.RunID, observer: observer,
	}
}

func (o *runtimeObserver) Observe(ctx context.Context, fact engine.Fact) {
	o.mu.Lock()
	if fact.Sequence > o.lastSequence {
		o.lastSequence = fact.Sequence
	}
	if fact.Kind == engine.FactRunCompleted || fact.Kind == engine.FactRunError {
		copy := fact
		o.terminal = &copy
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()
	o.dispatch(ctx, fact)
}

func (o *runtimeObserver) dispatch(ctx context.Context, fact engine.Fact) {
	correlated := RuntimeFact{SessionID: o.sessionID, RunID: o.runID, Fact: fact}
	if o.observer != nil {
		o.notifyObserver(ctx, correlated)
	}
	o.mu.Lock()
	if fact.Kind == engine.FactMessage {
		o.committedMessage = fact.Content
	}
	if fact.ArtifactPath != "" {
		o.artifactPaths = append(o.artifactPaths, fact.ArtifactPath)
	}
	o.mu.Unlock()
	if o.artifacts != nil {
		o.recordBestEffort("artifact", func() error { return o.artifacts.RecordArtifact(ctx, correlated) })
	}
	if o.telemetry != nil {
		o.recordBestEffort("telemetry", func() error { return o.telemetry.RecordTelemetry(ctx, correlated) })
	}
}

func (o *runtimeObserver) notifyObserver(ctx context.Context, fact RuntimeFact) {
	defer func() {
		if recovered := recover(); recovered != nil {
			o.addWarning(RunWarning{
				Sink: "observer", Operation: "observe_fact", Error: fmt.Sprintf("panic: %v", recovered),
			})
		}
	}()
	o.observer.ObserveRunFact(ctx, fact)
}

func (o *runtimeObserver) recordBestEffort(sink string, record func() error) {
	var recordErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				recordErr = fmt.Errorf("panic: %v", recovered)
			}
		}()
		recordErr = record()
	}()
	if recordErr != nil {
		o.addWarning(RunWarning{Sink: sink, Operation: "record_fact", Error: recordErr.Error()})
	}
}

func (o *runtimeObserver) observeFailure(ctx context.Context, err error) {
	o.mu.Lock()
	sequence := o.lastSequence
	o.mu.Unlock()
	if sequence == 0 {
		o.Observe(ctx, engine.Fact{Kind: engine.FactRunStarted, Sequence: 1})
		sequence = 1
	}
	o.Observe(ctx, engine.Fact{
		Kind: engine.FactRunError, Sequence: sequence + 1, Content: err.Error(), IsError: true,
	})
}

func (o *runtimeObserver) finish(ctx context.Context, finalErr error, finishFailed bool) {
	o.mu.Lock()
	terminal := o.terminal
	o.terminal = nil
	if finishFailed {
		sequence := o.lastSequence + 1
		if terminal != nil {
			sequence = terminal.Sequence
		}
		terminal = &engine.Fact{
			Kind: engine.FactRunError, Sequence: sequence, Content: finalErr.Error(), IsError: true,
		}
	}
	o.mu.Unlock()
	if terminal != nil {
		o.dispatch(ctx, *terminal)
	}
}

func (o *runtimeObserver) addWarning(warning RunWarning) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.warnings = append(o.warnings, warning)
}

func (o *runtimeObserver) result(outcome engine.RunOutcome) RunResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	return RunResult{
		SessionID: o.sessionID, RunID: o.runID, Outcome: outcome,
		ArtifactPaths:    append([]string(nil), o.artifactPaths...),
		Warnings:         append([]RunWarning(nil), o.warnings...),
		CommittedMessage: o.committedMessage,
	}
}

func cloneRunAssembly(request RunAssembly) RunAssembly {
	request.AllowedTools = cloneToolNames(request.AllowedTools)
	return request
}

func isNilRuntimeDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
