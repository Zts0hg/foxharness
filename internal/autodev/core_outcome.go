package autodev

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/provider"
)

// CoreOutcomeStatus is the closed terminal vocabulary for one core attempt.
type CoreOutcomeStatus string

const (
	CoreOutcomeSucceeded     CoreOutcomeStatus = "succeeded"
	CoreOutcomeFailed        CoreOutcomeStatus = "failed"
	CoreOutcomeCancelled     CoreOutcomeStatus = "cancelled"
	CoreOutcomeTurnExhausted CoreOutcomeStatus = "turn_exhausted"
	CoreOutcomeStartFailed   CoreOutcomeStatus = "start_failed"
)

// CoreRetryClass defines whether and how an unverified attempt may continue.
type CoreRetryClass string

const (
	CoreRetryNever       CoreRetryClass = "never"
	CoreRetrySameRunner  CoreRetryClass = "same_runner"
	CoreRetryFreshRunner CoreRetryClass = "fresh_runner"
)

// CoreAttempt is the durable identity supplied before one core side effect.
type CoreAttempt struct {
	AttemptID     string
	CorrelationID string
	Ordinal       int
	Prompt        string
}

// CoreLifecycleEvidence records which run-owned lifecycle boundaries exist.
type CoreLifecycleEvidence struct {
	RunStarted         bool
	PostRunEstablished bool
	DrainCompleted     bool
}

// CoreOutcome is the single terminal return value of CoreRunner.Run.
type CoreOutcome struct {
	Attempt        CoreAttempt
	Status         CoreOutcomeStatus
	SessionID      string
	RunID          string
	PartialMessage string
	Cause          error
	RetryClass     CoreRetryClass
	Lifecycle      CoreLifecycleEvidence
}

// CorePanicError reports that a CoreRunner violated its terminal-outcome
// contract by panicking across the control-plane boundary.
type CorePanicError struct {
	Value any
}

// Error implements error.
func (e *CorePanicError) Error() string {
	return fmt.Sprintf("core runner panicked before returning a terminal outcome: %v", e.Value)
}

// Validate rejects ambiguous or internally contradictory terminal outcomes.
func (o CoreOutcome) Validate() error {
	if o.Attempt.AttemptID == "" || o.Attempt.CorrelationID == "" || o.Attempt.Ordinal <= 0 {
		return errors.New("core outcome requires attempt, correlation, and ordinal identity")
	}
	switch o.Status {
	case CoreOutcomeSucceeded:
		if o.Cause != nil || o.RetryClass != CoreRetryNever || !o.Lifecycle.RunStarted {
			return errors.New("succeeded core outcome has contradictory failure or lifecycle evidence")
		}
	case CoreOutcomeFailed, CoreOutcomeTurnExhausted:
		if o.Cause == nil || !o.Lifecycle.RunStarted {
			return errors.New("started non-success core outcome requires a cause and run-start evidence")
		}
	case CoreOutcomeCancelled:
		if o.Cause == nil || o.RetryClass != CoreRetryNever {
			return errors.New("cancelled core outcome cannot be retryable")
		}
		if !o.Lifecycle.RunStarted && (o.SessionID != "" || o.RunID != "" || o.PartialMessage != "" || o.Lifecycle.PostRunEstablished) {
			return errors.New("pre-start cancellation cannot carry started-run evidence")
		}
	case CoreOutcomeStartFailed:
		if o.Cause == nil || o.Lifecycle.RunStarted || o.SessionID != "" || o.RunID != "" || o.PartialMessage != "" {
			return errors.New("start-failed core outcome cannot carry started-run evidence")
		}
	default:
		return fmt.Errorf("unknown core outcome status %q", o.Status)
	}
	if o.Lifecycle.PostRunEstablished && !o.Lifecycle.RunStarted {
		return errors.New("post-run lifecycle cannot exist before run start")
	}
	if o.Lifecycle.DrainCompleted && !o.Lifecycle.RunStarted {
		return errors.New("drain evidence cannot exist before run start")
	}
	if o.Lifecycle.RunStarted && (o.SessionID == "" || o.RunID == "") {
		return errors.New("started core outcome requires session and run identity")
	}
	if o.Status != CoreOutcomeSucceeded && o.RetryClass != CoreRetryNever &&
		o.RetryClass != CoreRetrySameRunner && o.RetryClass != CoreRetryFreshRunner {
		return fmt.Errorf("unknown core retry class %q", o.RetryClass)
	}
	return nil
}

// CoreOutcomeError retains one terminal outcome while preserving its cause.
type CoreOutcomeError struct {
	Outcome  CoreOutcome
	Verified bool
	Err      error
}

func (e *CoreOutcomeError) Error() string {
	if e == nil {
		return "core attempt failed"
	}
	cause := e.Err
	if cause == nil {
		cause = e.Outcome.Cause
	}
	if cause == nil {
		return fmt.Sprintf("core attempt %s ended with status %s", e.Outcome.Attempt.AttemptID, e.Outcome.Status)
	}
	return cause.Error()
}

func (e *CoreOutcomeError) Unwrap() error {
	if e == nil {
		return nil
	}
	if e.Err != nil {
		return e.Err
	}
	return e.Outcome.Cause
}

// CoreReviewEvidence prevents a non-success partial from being described as
// the core's successful final answer at the Engineer boundary.
type CoreReviewEvidence struct {
	Status        CoreOutcomeStatus
	Message       string
	SessionID     string
	RunID         string
	AttemptID     string
	CorrelationID string
	Partial       bool
	Cause         error
}

func (o CoreOutcome) reviewEvidence() CoreReviewEvidence {
	partial := o.Status != CoreOutcomeSucceeded
	return CoreReviewEvidence{
		Status: o.Status, Message: o.PartialMessage,
		SessionID: o.SessionID, RunID: o.RunID,
		AttemptID: o.Attempt.AttemptID, CorrelationID: o.Attempt.CorrelationID,
		Partial: partial, Cause: o.Cause,
	}
}

type coreRetryClassifier interface {
	CoreRetryClass() CoreRetryClass
}

type runtimeErrorClassifier interface {
	RuntimeErrorKind() string
}

func runtimeErrorKind(err error) string {
	var classifier runtimeErrorClassifier
	if errors.As(err, &classifier) {
		return classifier.RuntimeErrorKind()
	}
	return ""
}

func retryClassFromError(ctx context.Context, err error) CoreRetryClass {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return CoreRetryNever
	}
	var classifier coreRetryClassifier
	if errors.As(err, &classifier) {
		return classifier.CoreRetryClass()
	}
	if runtimeErrorKind(err) == "turn_limit" || provider.IsRetryableProviderError(ctx, err) {
		return CoreRetrySameRunner
	}
	return CoreRetryFreshRunner
}

// ClassifyCoreError maps the existing runtime error taxonomy into the closed
// Autodev terminal and retry vocabulary.
func ClassifyCoreError(ctx context.Context, err error, started bool) (CoreOutcomeStatus, CoreRetryClass) {
	if err == nil {
		return CoreOutcomeSucceeded, CoreRetryNever
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		return CoreOutcomeCancelled, CoreRetryNever
	}
	if runtimeErrorKind(err) == "turn_limit" {
		return CoreOutcomeTurnExhausted, CoreRetrySameRunner
	}
	if !started {
		return CoreOutcomeStartFailed, retryClassFromError(ctx, err)
	}
	return CoreOutcomeFailed, retryClassFromError(ctx, err)
}

func newCoreAttempt(sc *StageContext, prompt string) CoreAttempt {
	sc.CoreAttemptOrdinal++
	owner := strings.TrimSpace(string(sc.ItemID))
	if owner == "" {
		owner = strings.TrimSpace(sc.Slug)
	}
	if owner == "" {
		owner = "anonymous"
	}
	id := fmt.Sprintf("core:%s:%s:%d", owner, sc.Stage, sc.CoreAttemptOrdinal)
	return CoreAttempt{AttemptID: id, CorrelationID: id, Ordinal: sc.CoreAttemptOrdinal, Prompt: prompt}
}

func runningCoreAttemptRecord(sc *StageContext, attempt CoreAttempt) CoreAttemptRecord {
	return CoreAttemptRecord{
		AttemptID: attempt.AttemptID, CorrelationID: attempt.CorrelationID,
		Stage: PipelineStage(sc.Stage), Ordinal: attempt.Ordinal, State: CoreAttemptRunning,
	}
}

func terminalCoreAttemptRecord(sc *StageContext, outcome CoreOutcome) CoreAttemptRecord {
	record := CoreAttemptRecord{
		AttemptID: outcome.Attempt.AttemptID, CorrelationID: outcome.Attempt.CorrelationID,
		Stage: PipelineStage(sc.Stage), Ordinal: outcome.Attempt.Ordinal, State: CoreAttemptTerminal,
		OutcomeStatus: outcome.Status, SessionID: outcome.SessionID, RunID: outcome.RunID,
		RetryClass: outcome.RetryClass,
	}
	if outcome.Cause != nil {
		record.Cause = outcome.Cause.Error()
	}
	return record
}

func bindCoreAttemptRecorder(sc *StageContext, item *LedgerItem, commit func(string, func(*LedgerItem)) error) {
	sc.CoreAttemptOrdinal = lastCoreAttemptOrdinal(*item)
	sc.RecordCoreAttempt = func(attempt CoreAttemptRecord) error {
		candidate := cloneLedgerItem(*item)
		if err := updateCoreAttemptRecord(&candidate, attempt); err != nil {
			return err
		}
		operation := "core-attempt-" + attempt.AttemptID + "-" + string(attempt.State)
		var updateErr error
		if err := commit(operation, func(authoritative *LedgerItem) {
			updateErr = updateCoreAttemptRecord(authoritative, attempt)
		}); err != nil {
			return err
		}
		if updateErr != nil {
			return updateErr
		}
		*item = candidate
		return nil
	}
}
