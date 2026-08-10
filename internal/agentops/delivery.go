package agentops

import (
	"errors"
	"fmt"
	"log"
)

const maxAgentOpsTextRunes = 1800

/* DeliveryStage identifies the user-visible delivery phase that failed. */
type DeliveryStage string

const (
	DeliveryStageSession      DeliveryStage = "session"
	DeliveryStageFinal        DeliveryStage = "final"
	DeliveryStageFailure      DeliveryStage = "failure"
	DeliveryStagePanicFailure DeliveryStage = "panic_failure"
	DeliveryStageCancellation DeliveryStage = "cancellation"
)

/* DeliveryFailure records a failed transport operation with task correlation. */
type DeliveryFailure struct {
	TaskID string
	ChatID string
	Stage  DeliveryStage
	Cause  error
}

/* DeliveryFailureObserver receives delivery failures and must not block. */
type DeliveryFailureObserver interface {
	ObserveDeliveryFailure(DeliveryFailure)
}

type loggingDeliveryFailureObserver struct {
	logger *log.Logger
}

/* NewLoggingDeliveryFailureObserver creates the production logging observer. */
func NewLoggingDeliveryFailureObserver(logger *log.Logger) DeliveryFailureObserver {
	if logger == nil {
		logger = log.Default()
	}
	return loggingDeliveryFailureObserver{logger: logger}
}

func (o loggingDeliveryFailureObserver) ObserveDeliveryFailure(failure DeliveryFailure) {
	o.logger.Printf(
		"[AgentOps Delivery] task=%s chat=%s stage=%s failed: %v",
		failure.TaskID,
		failure.ChatID,
		failure.Stage,
		failure.Cause,
	)
}

func truncateAgentOpsText(text string) string {
	runes := []rune(text)
	if len(runes) <= maxAgentOpsTextRunes {
		return text
	}
	suffix := fmt.Sprintf("\n... (已截断，原始内容约 %d 字节)", len(text))
	suffixRunes := []rune(suffix)
	return string(runes[:maxAgentOpsTextRunes-len(suffixRunes)]) + suffix
}

var errMessengerUnavailable = errors.New("AgentOps messenger is unavailable")
