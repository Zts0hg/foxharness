package autodev

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
)

type outcomeCore struct {
	outcomes []CoreOutcome
	attempts []CoreAttempt
	drains   int
	replaces int
	onRun    func()
}

func (c *outcomeCore) Run(_ context.Context, attempt CoreAttempt, _ CoreReporter) CoreOutcome {
	c.attempts = append(c.attempts, attempt)
	if c.onRun != nil {
		c.onRun()
	}
	outcome := c.outcomes[0]
	c.outcomes = c.outcomes[1:]
	if outcome.Attempt.AttemptID == "" {
		outcome.Attempt = attempt
	}
	return outcome
}

func (c *outcomeCore) Drain(context.Context) error { c.drains++; return nil }
func (*outcomeCore) Close(context.Context) error   { return nil }
func (*outcomeCore) SetUserAsker(QuestionAsker)    {}
func (*outcomeCore) SetModel(string) error         { return nil }
func (*outcomeCore) WorkDir() string               { return "" }
func (*outcomeCore) StagePrompt(context.Context, string, string) (string, error) {
	return "seed", nil
}
func (c *outcomeCore) Replace(context.Context) error { c.replaces++; return nil }

type outcomeReviewEngineer struct {
	evidence []CoreReviewEvidence
	reviews  []string
}

func (*outcomeReviewEngineer) Decide(context.Context, []Question, StageContext) ([]Answer, error) {
	return nil, nil
}
func (*outcomeReviewEngineer) Reply(context.Context, string, StageContext) (string, error) {
	return "", nil
}
func (e *outcomeReviewEngineer) Review(_ context.Context, evidence CoreReviewEvidence, _ string, _ StageContext) (string, error) {
	e.evidence = append(e.evidence, evidence)
	if len(e.reviews) == 0 {
		return "", nil
	}
	review := e.reviews[0]
	e.reviews = e.reviews[1:]
	return review, nil
}

func TestDVAUT010CoreOutcomeRetainsTypedTerminalCorrelation(t *testing.T) {
	cause := &engine.TurnLimitError{MaxTurns: 3}
	outcome := CoreOutcome{
		Attempt:        CoreAttempt{AttemptID: "attempt-1", CorrelationID: "core:item-1:implement:1", Ordinal: 1},
		Status:         CoreOutcomeTurnExhausted,
		SessionID:      "session-1",
		RunID:          "run-1",
		PartialMessage: "committed assistant evidence",
		Cause:          cause,
		RetryClass:     CoreRetrySameRunner,
		Lifecycle: CoreLifecycleEvidence{
			RunStarted:         true,
			PostRunEstablished: true,
		},
	}
	if err := outcome.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want valid typed outcome", err)
	}
	err := &CoreOutcomeError{Outcome: outcome}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(%v, cause) = false", err)
	}
	var turnLimit *engine.TurnLimitError
	if !errors.As(err, &turnLimit) || turnLimit.MaxTurns != 3 {
		t.Fatalf("errors.As(%v) = %#v, want retained TurnLimitError", err, turnLimit)
	}
}

type coreStatusCodeError struct{ StatusCode int }

func (e *coreStatusCodeError) Error() string { return "provider status" }

func TestDVAUT010CoreErrorClassificationClosesRetryPolicy(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		started   bool
		wantState CoreOutcomeStatus
		wantRetry CoreRetryClass
	}{
		{name: "success", started: true, wantState: CoreOutcomeSucceeded, wantRetry: CoreRetryNever},
		{name: "provider", err: &coreStatusCodeError{StatusCode: 503}, started: true, wantState: CoreOutcomeFailed, wantRetry: CoreRetrySameRunner},
		{name: "tool", err: &retryHintError{class: CoreRetrySameRunner, err: errors.New("tool failed")}, started: true, wantState: CoreOutcomeFailed, wantRetry: CoreRetrySameRunner},
		{name: "turn-limit", err: &engine.TurnLimitError{MaxTurns: 3}, started: true, wantState: CoreOutcomeTurnExhausted, wantRetry: CoreRetrySameRunner},
		{name: "persistence", err: errors.New("session persistence failed"), started: true, wantState: CoreOutcomeFailed, wantRetry: CoreRetryFreshRunner},
		{name: "start-failure", err: errors.New("session creation failed"), wantState: CoreOutcomeStartFailed, wantRetry: CoreRetryFreshRunner},
		{name: "cancellation", err: context.Canceled, started: true, wantState: CoreOutcomeCancelled, wantRetry: CoreRetryNever},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, retryClass := ClassifyCoreError(context.Background(), tc.err, tc.started)
			if status != tc.wantState || retryClass != tc.wantRetry {
				t.Fatalf("ClassifyCoreError() = %s/%s, want %s/%s", status, retryClass, tc.wantState, tc.wantRetry)
			}
		})
	}
}

func TestDVAUT010AttemptHistoryAcceptsExactlyOneTerminalOutcome(t *testing.T) {
	item := LedgerItem{ItemID: "item-1"}
	running := CoreAttemptRecord{
		AttemptID: "core:item-1:implement-tasks:1", CorrelationID: "core:item-1:implement-tasks:1",
		Stage: StageImplementTasks, Ordinal: 1, State: CoreAttemptRunning,
	}
	if err := updateCoreAttemptRecord(&item, running); err != nil {
		t.Fatal(err)
	}
	terminal := running
	terminal.State = CoreAttemptTerminal
	terminal.OutcomeStatus = CoreOutcomeFailed
	terminal.RetryClass = CoreRetryFreshRunner
	terminal.Cause = "corrupt state"
	if err := updateCoreAttemptRecord(&item, terminal); err != nil {
		t.Fatal(err)
	}
	conflict := terminal
	conflict.OutcomeStatus = CoreOutcomeSucceeded
	conflict.RetryClass = CoreRetryNever
	conflict.Cause = ""
	if err := updateCoreAttemptRecord(&item, conflict); err == nil {
		t.Fatal("second terminal outcome rewrote durable attempt history")
	}
}

func TestDVAUT010StageMachineRejectsMismatchedAttemptCorrelation(t *testing.T) {
	core := &outcomeCore{outcomes: []CoreOutcome{{
		Attempt: CoreAttempt{AttemptID: "wrong", CorrelationID: "wrong", Ordinal: 99, Prompt: "work"},
		Status:  CoreOutcomeSucceeded, SessionID: "session-1", RunID: "run-1",
		RetryClass: CoreRetryNever, Lifecycle: CoreLifecycleEvidence{RunStarted: true},
	}}}
	verified := 0
	stage := Stage{
		Name:   "implement-tasks",
		Prompt: func(*StageContext) string { return "work" },
		Verify: func(context.Context, *StageContext) (bool, string) {
			verified++
			return true, ""
		},
	}
	err := NewStageMachine(&outcomeReviewEngineer{}, NewTerminalReporter(io.Discard)).RunStep(
		context.Background(), core, &StageContext{ItemID: "item-1"}, stage)
	var outcomeErr *CoreOutcomeError
	if !errors.As(err, &outcomeErr) || !strings.Contains(err.Error(), "mismatched attempt correlation") {
		t.Fatalf("RunStep error = %#v, want correlated contract failure", err)
	}
	if verified != 1 || core.drains != 1 {
		t.Fatalf("Verify/drain calls = %d/%d after a started attempt", verified, core.drains)
	}
}

func TestDVAUT010TerminalPersistenceFailureStillVerifiesAndRetainsOutcome(t *testing.T) {
	cause := errors.New("provider failed")
	commitCause := errors.New("disk full")
	core := &outcomeCore{outcomes: []CoreOutcome{{
		Status: CoreOutcomeFailed, Cause: cause, RetryClass: CoreRetrySameRunner,
		SessionID: "session-1", RunID: "run-1", PartialMessage: "committed partial",
		Lifecycle: CoreLifecycleEvidence{RunStarted: true},
	}}}
	verified := 0
	sc := &StageContext{ItemID: "item-1", Slug: "item-1"}
	sc.RecordCoreAttempt = func(record CoreAttemptRecord) error {
		if record.State == CoreAttemptTerminal {
			return commitCause
		}
		return nil
	}
	stage := Stage{
		Name:   "implement-tasks",
		Prompt: func(*StageContext) string { return "run" },
		Verify: func(context.Context, *StageContext) (bool, string) {
			verified++
			return false, "artifact absent"
		},
	}
	err := NewStageMachine(&outcomeReviewEngineer{}, NewTerminalReporter(io.Discard)).RunStep(context.Background(), core, sc, stage)
	var outcomeErr *CoreOutcomeError
	var ledgerErr *LedgerCommitError
	if !errors.As(err, &outcomeErr) || !errors.As(err, &ledgerErr) || !errors.Is(err, cause) || !errors.Is(err, commitCause) {
		t.Fatalf("RunStep error = %#v, want correlated outcome plus ledger failure", err)
	}
	if verified != 1 || !outcomeErr.Outcome.Lifecycle.DrainCompleted || outcomeErr.Outcome.PartialMessage != "committed partial" {
		t.Fatalf("verified/outcome = %d/%#v", verified, outcomeErr.Outcome)
	}
}

func TestDVAUT010EveryStartedTerminalOutcomeDrainsThenVerifies(t *testing.T) {
	statuses := []CoreOutcomeStatus{
		CoreOutcomeSucceeded,
		CoreOutcomeFailed,
		CoreOutcomeCancelled,
		CoreOutcomeTurnExhausted,
	}
	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			cause := error(nil)
			retryClass := CoreRetryNever
			if status != CoreOutcomeSucceeded {
				cause = errors.New(string(status))
			}
			if status == CoreOutcomeFailed || status == CoreOutcomeTurnExhausted {
				retryClass = CoreRetrySameRunner
			}
			core := &outcomeCore{outcomes: []CoreOutcome{{
				Status:     status,
				Cause:      cause,
				RetryClass: retryClass,
				SessionID:  "session-1",
				RunID:      "run-1",
				Lifecycle: CoreLifecycleEvidence{
					RunStarted: true,
				},
			}}}
			verified := 0
			stage := Stage{
				Name:   "implement-tasks",
				Prompt: func(*StageContext) string { return "run" },
				Verify: func(context.Context, *StageContext) (bool, string) {
					if core.drains != 1 {
						t.Fatalf("Verify ran after %d drains, want exactly one", core.drains)
					}
					verified++
					return true, ""
				},
			}
			err := NewStageMachine(&outcomeReviewEngineer{}, NewTerminalReporter(io.Discard)).RunStep(
				context.Background(), core, &StageContext{ItemID: "item-1", Slug: "item-1"}, stage)
			if status == CoreOutcomeCancelled {
				var outcomeErr *CoreOutcomeError
				if !errors.As(err, &outcomeErr) || !outcomeErr.Verified || !errors.Is(err, cause) {
					t.Fatalf("cancelled RunStep error = %#v, want verified correlated outcome", err)
				}
			} else if err != nil {
				t.Fatalf("RunStep(%s) = %v, verified ground truth must complete", status, err)
			}
			if verified != 1 {
				t.Fatalf("Verify calls = %d, want 1", verified)
			}
		})
	}
}

func TestDVAUT010CancellationUsesFreshVerificationContextAndNeverRetries(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	core := &outcomeCore{outcomes: []CoreOutcome{{
		Status:     CoreOutcomeCancelled,
		Cause:      context.Canceled,
		RetryClass: CoreRetryNever,
		SessionID:  "session-1",
		RunID:      "run-1",
		Lifecycle: CoreLifecycleEvidence{
			RunStarted: true,
		},
	}}, onRun: cancel}
	verifiedWithLiveContext := false
	stage := Stage{
		Name:   "implement-tasks",
		Prompt: func(*StageContext) string { return "run" },
		Verify: func(verifyCtx context.Context, _ *StageContext) (bool, string) {
			verifiedWithLiveContext = verifyCtx.Err() == nil
			return false, "artifact absent"
		},
	}
	err := NewStageMachine(&outcomeReviewEngineer{}, NewTerminalReporter(io.Discard)).RunStep(
		ctx, core, &StageContext{ItemID: "item-1", Slug: "item-1"}, stage)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("RunStep error = %v, want context.Canceled", err)
	}
	if !verifiedWithLiveContext {
		t.Fatal("cancellation reconciliation inherited the cancelled parent context")
	}
	if len(core.attempts) != 1 || core.replaces != 0 {
		t.Fatalf("attempts/replacements = %d/%d, cancellation must never retry", len(core.attempts), core.replaces)
	}
}

func TestDVAUT010StartFailureReturnsOneOutcomeWithoutStageVerification(t *testing.T) {
	cause := errors.New("runner could not establish a session")
	core := &outcomeCore{outcomes: []CoreOutcome{{
		Status: CoreOutcomeStartFailed, Cause: cause, RetryClass: CoreRetryNever,
	}}}
	verified := 0
	stage := Stage{
		Name:   "implement-tasks",
		Prompt: func(*StageContext) string { return "run" },
		Verify: func(context.Context, *StageContext) (bool, string) {
			verified++
			return true, ""
		},
	}
	err := NewStageMachine(&outcomeReviewEngineer{}, NewTerminalReporter(io.Discard)).RunStep(
		context.Background(), core, &StageContext{ItemID: "item-1", Slug: "item-1"}, stage)
	var outcomeErr *CoreOutcomeError
	if !errors.As(err, &outcomeErr) || outcomeErr.Outcome.Status != CoreOutcomeStartFailed || !errors.Is(err, cause) {
		t.Fatalf("RunStep error = %#v, want correlated start-failed outcome", err)
	}
	if verified != 0 || core.drains != 0 || len(core.attempts) != 1 {
		t.Fatalf("verify/drain/attempts = %d/%d/%d, want 0/0/1", verified, core.drains, len(core.attempts))
	}
}

func TestDVAUT010RetryPersistsCorrelationAndLabelsPartialBeforeFreshRunner(t *testing.T) {
	cause := &retryHintError{class: CoreRetryFreshRunner, err: errors.New("session state is corrupt")}
	core := &outcomeCore{outcomes: []CoreOutcome{
		{
			Status:         CoreOutcomeFailed,
			SessionID:      "session-1",
			RunID:          "run-1",
			PartialMessage: "committed partial",
			Cause:          cause,
			RetryClass:     CoreRetryFreshRunner,
			Lifecycle:      CoreLifecycleEvidence{RunStarted: true},
		},
		{Status: CoreOutcomeSucceeded, SessionID: "session-2", RunID: "run-2", RetryClass: CoreRetryNever, Lifecycle: CoreLifecycleEvidence{RunStarted: true}},
	}}
	engineer := &outcomeReviewEngineer{reviews: []string{"repair from clean session"}}
	var records []CoreAttemptRecord
	verified := 0
	sc := &StageContext{
		ItemID: "item-1",
		Slug:   "item-1",
		RecordCoreAttempt: func(record CoreAttemptRecord) error {
			records = append(records, record)
			return nil
		},
	}
	stage := Stage{
		Name:   "implement-tasks",
		Prompt: func(*StageContext) string { return "run" },
		Verify: func(context.Context, *StageContext) (bool, string) {
			verified++
			return verified == 2, "artifact absent"
		},
	}
	if err := NewStageMachine(engineer, NewTerminalReporter(io.Discard)).RunStep(context.Background(), core, sc, stage); err != nil {
		t.Fatalf("RunStep() = %v", err)
	}
	if core.replaces != 1 {
		t.Fatalf("fresh-runner replacements = %d, want 1", core.replaces)
	}
	if len(engineer.evidence) != 1 || !engineer.evidence[0].Partial ||
		!strings.Contains(engineer.evidence[0].Message, "committed partial") {
		t.Fatalf("Engineer evidence = %#v, want explicitly non-final partial evidence", engineer.evidence)
	}
	if len(records) != 4 {
		t.Fatalf("attempt records = %#v, want running+terminal for two attempts", records)
	}
	for i := 0; i < len(records); i += 2 {
		if records[i].State != CoreAttemptRunning || records[i+1].State != CoreAttemptTerminal ||
			records[i].AttemptID != records[i+1].AttemptID || records[i].CorrelationID == "" {
			t.Fatalf("attempt record pair %d = %#v / %#v", i/2, records[i], records[i+1])
		}
	}
	if records[2].AttemptID == records[0].AttemptID {
		t.Fatal("retry reused the prior attempt identity")
	}
}

type retryHintError struct {
	class CoreRetryClass
	err   error
}

func (e *retryHintError) Error() string                  { return e.err.Error() }
func (e *retryHintError) Unwrap() error                  { return e.err }
func (e *retryHintError) CoreRetryClass() CoreRetryClass { return e.class }

type replacementCoreFactory struct {
	artifact bool
	created  []*replacementCore
}

func (f *replacementCoreFactory) New(_ context.Context, workDir, _ string) (CoreRunner, error) {
	core := &replacementCore{factory: f, workDir: workDir, generation: len(f.created) + 1}
	f.created = append(f.created, core)
	return core, nil
}

type replacementCore struct {
	factory    *replacementCoreFactory
	workDir    string
	generation int
	closed     bool
}

func (c *replacementCore) Run(_ context.Context, attempt CoreAttempt, _ CoreReporter) CoreOutcome {
	if c.generation == 1 {
		cause := &retryHintError{class: CoreRetryFreshRunner, err: errors.New("corrupt session state")}
		return CoreOutcome{
			Attempt: attempt, Status: CoreOutcomeFailed, SessionID: "session-1", RunID: "run-1",
			PartialMessage: "committed partial", Cause: cause, RetryClass: CoreRetryFreshRunner,
			Lifecycle: CoreLifecycleEvidence{RunStarted: true},
		}
	}
	c.factory.artifact = true
	return successfulCoreOutcome(attempt, "completed from fresh runner")
}
func (*replacementCore) Drain(context.Context) error   { return nil }
func (c *replacementCore) Close(context.Context) error { c.closed = true; return nil }
func (*replacementCore) SetUserAsker(QuestionAsker)    {}
func (*replacementCore) SetModel(string) error         { return nil }
func (c *replacementCore) WorkDir() string             { return c.workDir }
func (*replacementCore) StagePrompt(context.Context, string, string) (string, error) {
	return "seed", nil
}

func TestDVAUT010OrchestratorPersistsAttemptsAndReplacesFreshRunnerBeforeRetry(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, _, _ := testDeps(t, repoRoot, "## Retry item\n\n**Description**: recover safely\n")
	factory := &replacementCoreFactory{}
	deps.CoreFactory = factory
	deps.Engineer = &outcomeReviewEngineer{reviews: []string{"retry from a clean session"}}
	deps.BuildPipeline = func(PipelineDeps) []Stage {
		return []Stage{{
			Name:   "implement-tasks",
			Prompt: func(*StageContext) string { return "work" },
			Verify: func(context.Context, *StageContext) (bool, string) {
				return factory.artifact, "artifact absent"
			},
		}}
	}
	if err := New(deps).Run(context.Background()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(factory.created) != 2 || !factory.created[0].closed || !factory.created[1].closed {
		t.Fatalf("runner generations = %#v, want old closed before fresh runner and final close", factory.created)
	}
	ledger, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	items := ledger.selectByStatus(StatusDone)
	if len(items) != 1 {
		t.Fatalf("durable attempts = %#v", items)
	}
	var implementationAttempts []CoreAttemptRecord
	for i, record := range items[0].CoreAttempts {
		if record.State != CoreAttemptTerminal || record.AttemptID == "" || record.CorrelationID == "" {
			t.Fatalf("attempt[%d] = %#v", i, record)
		}
		if record.Stage == StageImplementTasks {
			implementationAttempts = append(implementationAttempts, record)
		}
	}
	if len(implementationAttempts) != 2 || implementationAttempts[0].RetryClass != CoreRetryFreshRunner ||
		implementationAttempts[1].OutcomeStatus != CoreOutcomeSucceeded {
		t.Fatalf("durable retry history = %#v", implementationAttempts)
	}
}

type cancellingVerifiedCore struct {
	stubCore
	cancel   context.CancelFunc
	artifact *bool
}

func (c *cancellingVerifiedCore) Run(_ context.Context, attempt CoreAttempt, _ CoreReporter) CoreOutcome {
	*c.artifact = true
	c.cancel()
	return CoreOutcome{
		Attempt: attempt, Status: CoreOutcomeCancelled, SessionID: "session-1", RunID: "run-1", Cause: context.Canceled,
		RetryClass: CoreRetryNever, Lifecycle: CoreLifecycleEvidence{RunStarted: true},
	}
}

type cancellingVerifiedFactory struct{ core *cancellingVerifiedCore }

func (f cancellingVerifiedFactory) New(_ context.Context, workDir, _ string) (CoreRunner, error) {
	f.core.workDir = workDir
	return f.core, nil
}

func TestDVAUT010VerifiedCancellationIsDurableButStillStops(t *testing.T) {
	repoRoot := t.TempDir()
	deps, _, _, _, _ := testDeps(t, repoRoot, "## Cancelled item\n\n**Description**: verify before stop\n")
	ctx, cancel := context.WithCancel(context.Background())
	artifact := false
	core := &cancellingVerifiedCore{cancel: cancel, artifact: &artifact}
	deps.CoreFactory = cancellingVerifiedFactory{core: core}
	deps.BuildPipeline = func(PipelineDeps) []Stage {
		return []Stage{{
			Name:   "implement-tasks",
			Prompt: func(*StageContext) string { return "work" },
			Verify: func(verifyCtx context.Context, _ *StageContext) (bool, string) {
				if verifyCtx.Err() != nil {
					t.Fatal("verified cancellation inherited cancelled context")
				}
				return artifact, "artifact absent"
			},
		}}
	}
	if err := New(deps).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want context.Canceled after durable verification", err)
	}
	ledger, err := LoadLedger(filepath.Join(repoRoot, ".foxharness", "autodev-state.json"), newTestClock())
	if err != nil {
		t.Fatal(err)
	}
	items := ledger.InProgress()
	if len(items) != 1 || items[0].Stage != StageImplementTasks || items[0].StageState != StageStateVerified {
		t.Fatalf("ledger after verified cancellation = %#v", items)
	}
	if len(items[0].CoreAttempts) != 1 || items[0].CoreAttempts[0].OutcomeStatus != CoreOutcomeCancelled {
		t.Fatalf("cancelled attempt history = %#v", items[0].CoreAttempts)
	}
}
