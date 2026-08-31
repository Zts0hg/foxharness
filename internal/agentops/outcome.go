package agentops

import "log"

/* TaskOutcomeStatus identifies the terminal state of one accepted task. */
type TaskOutcomeStatus string

const (
	/* TaskOutcomeCompleted identifies a task that produced its normal result. */
	TaskOutcomeCompleted TaskOutcomeStatus = "completed"
	/* TaskOutcomeFailed identifies a task that terminated without a result. */
	TaskOutcomeFailed TaskOutcomeStatus = "failed"
	/* TaskOutcomeCancelled identifies a task stopped by cancellation or timeout. */
	TaskOutcomeCancelled TaskOutcomeStatus = "cancelled"
)

/* TaskOutcomeReason identifies why a task entered its terminal state. */
type TaskOutcomeReason string

const (
	TaskOutcomeReasonCompleted    TaskOutcomeReason = "completed"
	TaskOutcomeReasonFailure      TaskOutcomeReason = "failure"
	TaskOutcomeReasonTimeout      TaskOutcomeReason = "timeout"
	TaskOutcomeReasonCancellation TaskOutcomeReason = "cancellation"
	TaskOutcomeReasonPanic        TaskOutcomeReason = "panic"
)

/* TaskOutcome is the correlated terminal record emitted for an accepted task. */
type TaskOutcome struct {
	TaskID string
	ChatID string
	Status TaskOutcomeStatus
	Reason TaskOutcomeReason
	Error  string
}

/* TaskOutcomeObserver receives terminal task records and must not block. */
type TaskOutcomeObserver interface {
	ObserveTaskOutcome(TaskOutcome)
}

type loggingTaskOutcomeObserver struct{}

func (loggingTaskOutcomeObserver) ObserveTaskOutcome(outcome TaskOutcome) {
	log.Printf(
		"[AgentOps] task=%s chat=%s status=%s reason=%s error=%s",
		outcome.TaskID,
		outcome.ChatID,
		outcome.Status,
		outcome.Reason,
		outcome.Error,
	)
}
