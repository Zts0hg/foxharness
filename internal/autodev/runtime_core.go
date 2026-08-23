package autodev

import (
	"context"
	"errors"
	"reflect"
	"sync"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

// RuntimeCoreSession is the runtime capability required by one item-scoped core runner.
type RuntimeCoreSession interface {
	Run(context.Context, foxruntime.RunSpec) (foxruntime.RunResult, error)
}

// RuntimeCoreRunnerConfig binds an item-scoped runtime session to Autodev control ports.
type RuntimeCoreRunnerConfig struct {
	Session          RuntimeCoreSession
	BaseSpec         foxruntime.RunSpec
	WorkDir          string
	SetQuestionAsker func(QuestionAsker)
	SetModel         func(string) error
	StagePrompt      func(context.Context, string, string) (string, error)
	BeforeRun        func(context.Context, CoreAttempt) error
	AfterRun         func(context.Context, foxruntime.RunResult, error)
	Drain            func(context.Context) error
	Close            func(context.Context) error
}

// RuntimeCoreRunner adapts one long-lived runtime session to Autodev's item execution port.
type RuntimeCoreRunner struct {
	session RuntimeCoreSession
	config  RuntimeCoreRunnerConfig
	mu      sync.Mutex
	spec    foxruntime.RunSpec
}

// NewRuntimeCoreRunner validates and freezes one runtime-backed core adapter.
func NewRuntimeCoreRunner(config RuntimeCoreRunnerConfig) (*RuntimeCoreRunner, error) {
	if isNilRuntimeCoreSession(config.Session) {
		return nil, errors.New("autodev runtime session is required")
	}
	config.BaseSpec = cloneRuntimeCoreSpec(config.BaseSpec)
	return &RuntimeCoreRunner{session: config.Session, config: config, spec: config.BaseSpec}, nil
}

// Run executes one attempt through the shared runtime and returns one correlated outcome.
func (r *RuntimeCoreRunner) Run(ctx context.Context, attempt CoreAttempt, reporter CoreReporter) CoreOutcome {
	if r.config.BeforeRun != nil {
		if err := r.config.BeforeRun(ctx, attempt); err != nil {
			status, retryClass := classifyRuntimeCoreError(ctx, err, false, "")
			return CoreOutcome{Attempt: attempt, Status: status, Cause: err, RetryClass: retryClass}
		}
	}
	r.mu.Lock()
	spec := r.spec
	r.mu.Unlock()
	spec.Prompt = attempt.Prompt
	spec.Observer = foxruntime.RunObserverFunc(func(ctx context.Context, fact foxruntime.RuntimeFact) {
		forwardRuntimeCoreFact(ctx, reporter, fact)
	})

	result, runErr := r.session.Run(ctx, spec)
	started := result.RunID != ""
	partial := result.CommittedMessage
	if runErr == nil {
		partial = result.Outcome.FinalMessage
	}
	status, retryClass := classifyRuntimeCoreError(ctx, runErr, started, result.Outcome.ErrorKind)
	if status == CoreOutcomeStartFailed {
		result.SessionID, result.RunID, partial = "", "", ""
	}
	if reporter != nil && started {
		if runErr != nil {
			reporter.OnRunError(ctx, string(result.SessionID), string(result.RunID), runErr)
		} else {
			reporter.OnRunComplete(ctx, CoreRunResult{
				SessionID: string(result.SessionID), RunID: string(result.RunID), FinalMessage: result.Outcome.FinalMessage,
			})
		}
	}
	if r.config.AfterRun != nil {
		r.config.AfterRun(ctx, result, runErr)
	}
	return CoreOutcome{
		Attempt: attempt, Status: status, SessionID: string(result.SessionID), RunID: string(result.RunID),
		PartialMessage: partial, Cause: runErr, RetryClass: retryClass,
		Lifecycle: CoreLifecycleEvidence{RunStarted: started, PostRunEstablished: started},
	}
}

func forwardRuntimeCoreFact(ctx context.Context, reporter CoreReporter, fact foxruntime.RuntimeFact) {
	if reporter == nil {
		return
	}
	switch string(fact.Fact.Kind) {
	case "run_started":
		reporter.OnRunStart(ctx, string(fact.SessionID), string(fact.RunID))
	case "thinking":
		reporter.OnThinking(ctx, fact.Fact.Turn)
	case "compaction":
		reporter.OnCompaction(ctx, fact.Fact.Name)
	case "tool_call":
		reporter.OnToolCall(ctx, fact.Fact.Name, fact.Fact.Content)
	case "tool_result":
		reporter.OnToolResult(ctx, fact.Fact.Name, fact.Fact.Content, fact.Fact.IsError)
	case "message":
		reporter.OnMessage(ctx, fact.Fact.Content)
	}
}

func classifyRuntimeCoreError(ctx context.Context, err error, started bool, kind string) (CoreOutcomeStatus, CoreRetryClass) {
	if err == nil {
		return CoreOutcomeSucceeded, CoreRetryNever
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return CoreOutcomeCancelled, CoreRetryNever
	}
	if kind == "turn_limit" {
		return CoreOutcomeTurnExhausted, CoreRetrySameRunner
	}
	if !started {
		return CoreOutcomeStartFailed, retryClassFromError(ctx, err)
	}
	return CoreOutcomeFailed, retryClassFromError(ctx, err)
}

// Drain joins post-run work launched by completed item attempts.
func (r *RuntimeCoreRunner) Drain(ctx context.Context) error {
	if r.config.Drain == nil {
		return nil
	}
	return r.config.Drain(ctx)
}

// Close closes the item-scoped runtime and its post-run resources.
func (r *RuntimeCoreRunner) Close(ctx context.Context) error {
	if r.config.Close == nil {
		return nil
	}
	return r.config.Close(ctx)
}

// SetUserAsker installs the Engineer-mediated question port.
func (r *RuntimeCoreRunner) SetUserAsker(asker QuestionAsker) {
	if r.config.SetQuestionAsker != nil {
		r.config.SetQuestionAsker(asker)
	}
}

// SetModel switches the model used by future attempts.
func (r *RuntimeCoreRunner) SetModel(model string) error {
	if r.config.SetModel != nil {
		if err := r.config.SetModel(model); err != nil {
			return err
		}
	}
	r.mu.Lock()
	r.spec.Model = model
	r.mu.Unlock()
	return nil
}

// WorkDir returns the item worktree bound to this runner.
func (r *RuntimeCoreRunner) WorkDir() string { return r.config.WorkDir }

// StagePrompt delegates deterministic CodexSpec prompt materialization.
func (r *RuntimeCoreRunner) StagePrompt(ctx context.Context, command, args string) (string, error) {
	if r.config.StagePrompt == nil {
		return "", errors.New("autodev stage prompt materializer is required")
	}
	return r.config.StagePrompt(ctx, command, args)
}

func isNilRuntimeCoreSession(value RuntimeCoreSession) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func cloneRuntimeCoreSpec(spec foxruntime.RunSpec) foxruntime.RunSpec {
	spec.AllowedTools = cloneToolNames(spec.AllowedTools)
	if spec.MaxTurns != nil {
		value := *spec.MaxTurns
		spec.MaxTurns = &value
	}
	if spec.TaskTimeout != nil {
		value := *spec.TaskTimeout
		spec.TaskTimeout = &value
	}
	if spec.Thinking != nil {
		value := *spec.Thinking
		spec.Thinking = &value
	}
	if spec.ReadOnly != nil {
		value := *spec.ReadOnly
		spec.ReadOnly = &value
	}
	if spec.DelegationDepth != nil {
		value := *spec.DelegationDepth
		spec.DelegationDepth = &value
	}
	return spec
}

func cloneToolNames(tools []string) []string {
	if tools == nil {
		return nil
	}
	return append([]string{}, tools...)
}

var _ CoreRunner = (*RuntimeCoreRunner)(nil)
