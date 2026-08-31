package feishu

import (
	"errors"
	"log"
)

/* DeliveryStage identifies the user-visible delivery phase that failed. */
type DeliveryStage string

const (
	DeliveryStageReceipt      DeliveryStage = "receipt"
	DeliveryStageSession      DeliveryStage = "session"
	DeliveryStageLifecycle    DeliveryStage = "lifecycle"
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
		"[Feishu Delivery] task=%s chat=%s stage=%s failed: %v",
		failure.TaskID,
		failure.ChatID,
		failure.Stage,
		failure.Cause,
	)
}

var errMessengerUnavailable = errors.New("Feishu messenger is unavailable")
