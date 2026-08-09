package feishu

import "log"

/* TaskOutcomeStatus identifies the terminal state of one accepted task. */
type TaskOutcomeStatus string

const (
	/* TaskOutcomeFailed identifies a task that terminated without a result. */
	TaskOutcomeFailed TaskOutcomeStatus = "failed"
)

/* TaskOutcome is the correlated terminal record emitted for an accepted task. */
type TaskOutcome struct {
	TaskID string
	ChatID string
	Status TaskOutcomeStatus
	Error  string
}

/* TaskOutcomeObserver receives terminal task records and must not block. */
type TaskOutcomeObserver interface {
	ObserveTaskOutcome(TaskOutcome)
}

type loggingTaskOutcomeObserver struct{}

func (loggingTaskOutcomeObserver) ObserveTaskOutcome(outcome TaskOutcome) {
	log.Printf("[Feishu Runner] task=%s chat=%s status=%s error=%s", outcome.TaskID, outcome.ChatID, outcome.Status, outcome.Error)
}
