/*
Package agentops provides incident task policy, scheduling, presentation, and
the AgentOps-owned log-search capability over application execution ports.
*/
package agentops

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
)

// Messenger abstracts the ability to send a plain-text message to a chat
// identified by chatID.  Implementations are typically backed by an IM
// platform such as Feishu.
type Messenger interface {
	// SendText delivers text to the specified chat.  A non-nil error
	// indicates a delivery failure.
	SendText(ctx context.Context, chatID, text string) error
}

/* Runner owns AgentOps scheduling and terminal presentation over an application port. */
type Runner struct {
	executionFactory        TaskExecutionFactory
	messenger               Messenger
	maxConcurrentTasks      int
	taskTimeout             time.Duration
	runTask                 func(context.Context, Task) error
	taskOutcomeObserver     TaskOutcomeObserver
	deliveryFailureObserver DeliveryFailureObserver
}

/* WithDeliveryFailureObserver installs the non-blocking delivery failure observer. */
func (r *Runner) WithDeliveryFailureObserver(observer DeliveryFailureObserver) *Runner {
	r.deliveryFailureObserver = observer
	return r
}

const (
	defaultMaxConcurrentTasks  = 4
	defaultTaskTimeout         = 5 * time.Minute
	defaultTerminalSendTimeout = 10 * time.Second
)

/* NewRunner constructs an AgentOps control-plane runner over an application execution factory. */
func NewRunner(executionFactory TaskExecutionFactory, messenger Messenger) *Runner {
	return &Runner{
		executionFactory:    executionFactory,
		messenger:           messenger,
		maxConcurrentTasks:  defaultMaxConcurrentTasks,
		taskTimeout:         defaultTaskTimeout,
		taskOutcomeObserver: loggingTaskOutcomeObserver{},
	}
}

// Start consumes every accepted task until the producer closes the channel and
// waits for all workers. Cancellation is propagated to each task but does not
// make the consumer abandon tasks already accepted by the upstream lifecycle.
func (r *Runner) Start(ctx context.Context, tasks <-chan Task) {
	permits := make(chan struct{}, r.concurrentTaskLimit())
	var workers sync.WaitGroup
	for task := range tasks {
		permits <- struct{}{}
		workers.Add(1)
		go func(task Task) {
			defer workers.Done()
			outcome := r.executeTask(ctx, task)
			<-permits
			r.completeTask(task, outcome)
		}(task)
	}
	workers.Wait()
}

// Run executes one task and publishes exactly one correlated terminal outcome.
func (r *Runner) Run(ctx context.Context, task Task) {
	r.completeTask(task, r.executeTask(ctx, task))
}

func (r *Runner) executeTask(ctx context.Context, task Task) (outcome TaskOutcome) {
	runCtx, cancel := context.WithTimeout(ctx, r.taskTimeoutOrDefault())
	defer cancel()
	outcome = TaskOutcome{
		TaskID: task.TaskID,
		ChatID: task.ChatID,
		Status: TaskOutcomeCompleted,
		Reason: TaskOutcomeReasonCompleted,
	}
	defer func() {
		if rec := recover(); rec != nil {
			outcome.Status = TaskOutcomeFailed
			outcome.Reason = TaskOutcomeReasonPanic
			outcome.Error = fmt.Sprintf("task panic: %v", rec)
		}
	}()
	if err := runCtx.Err(); err != nil {
		outcome.Error = err.Error()
		outcome.Status = TaskOutcomeCancelled
		if errors.Is(err, context.DeadlineExceeded) {
			outcome.Reason = TaskOutcomeReasonTimeout
		} else {
			outcome.Reason = TaskOutcomeReasonCancellation
		}
		return outcome
	}

	err := r.taskRunner()(runCtx, task)
	if err == nil {
		return outcome
	}
	outcome.Error = err.Error()
	switch {
	case errors.Is(runCtx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		outcome.Status = TaskOutcomeCancelled
		outcome.Reason = TaskOutcomeReasonTimeout
	case errors.Is(runCtx.Err(), context.Canceled), errors.Is(err, context.Canceled):
		outcome.Status = TaskOutcomeCancelled
		outcome.Reason = TaskOutcomeReasonCancellation
	default:
		outcome.Status = TaskOutcomeFailed
		outcome.Reason = TaskOutcomeReasonFailure
	}
	return outcome
}

func (r *Runner) completeTask(task Task, outcome TaskOutcome) {
	if outcome.Status != TaskOutcomeCompleted {
		log.Printf("[AgentOps] task=%s failed: %s", task.TaskID, outcome.Error)
	}
	r.observeTaskOutcome(outcome)
	if outcome.Status == TaskOutcomeCompleted {
		return
	}
	deliveryCtx, cancel := context.WithTimeout(context.Background(), defaultTerminalSendTimeout)
	defer cancel()
	r.sendTerminalFailure(deliveryCtx, task, outcome)
}

func (r *Runner) observeTaskOutcome(outcome TaskOutcome) {
	if r.taskOutcomeObserver == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[AgentOps] task=%s outcome observer panic recovered: %v", outcome.TaskID, rec)
		}
	}()
	r.taskOutcomeObserver.ObserveTaskOutcome(outcome)
}

func (r *Runner) sendTerminalFailure(ctx context.Context, task Task, outcome TaskOutcome) {
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[AgentOps] task=%s terminal delivery panic recovered: %v", task.TaskID, rec)
		}
	}()
	stage := DeliveryStageFailure
	switch outcome.Reason {
	case TaskOutcomeReasonPanic:
		stage = DeliveryStagePanicFailure
	case TaskOutcomeReasonTimeout, TaskOutcomeReasonCancellation:
		stage = DeliveryStageCancellation
	}
	_ = r.deliverTaskText(ctx, task, stage, fmt.Sprintf("AgentOps 任务失败： %s", outcome.Error))
}

func (r *Runner) deliverTaskText(ctx context.Context, task Task, stage DeliveryStage, text string) error {
	if isNilAgentOpsDependency(r.messenger) {
		r.observeDeliveryFailure(DeliveryFailure{TaskID: task.TaskID, ChatID: task.ChatID, Stage: stage, Cause: errMessengerUnavailable})
		return errMessengerUnavailable
	}
	err := r.messenger.SendText(ctx, task.ChatID, truncateAgentOpsText(text))
	if err != nil {
		r.observeDeliveryFailure(DeliveryFailure{TaskID: task.TaskID, ChatID: task.ChatID, Stage: stage, Cause: err})
	}
	return err
}

func (r *Runner) observeDeliveryFailure(failure DeliveryFailure) {
	if r.deliveryFailureObserver == nil {
		log.Printf("[AgentOps Delivery] task=%s chat=%s stage=%s failed: %v", failure.TaskID, failure.ChatID, failure.Stage, failure.Cause)
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[AgentOps] task=%s delivery observer panic recovered: %v", failure.TaskID, rec)
		}
	}()
	r.deliveryFailureObserver.ObserveDeliveryFailure(failure)
}

func (r *Runner) concurrentTaskLimit() int {
	if r.maxConcurrentTasks > 0 {
		return r.maxConcurrentTasks
	}
	return defaultMaxConcurrentTasks
}

func (r *Runner) taskTimeoutOrDefault() time.Duration {
	if r.taskTimeout > 0 {
		return r.taskTimeout
	}
	return defaultTaskTimeout
}

func (r *Runner) taskRunner() func(context.Context, Task) error {
	if r.runTask != nil {
		return r.runTask
	}
	return r.run
}

func (r *Runner) run(ctx context.Context, task Task) error {
	if isNilAgentOpsDependency(r.executionFactory) {
		return errors.New("AgentOps task execution factory is required")
	}
	prompt := BuildPrompt(task)
	prepared, err := r.executionFactory.PrepareTask(ctx, TaskExecutionRequest{Task: task, Prompt: prompt})
	if err != nil {
		return err
	}
	_ = r.deliverTaskText(
		ctx,
		task,
		DeliveryStageSession,
		fmt.Sprintf("已创建 AgentOps Session: %s\n开始分析。", prepared.Session.ID),
	)
	if prepared.Start == nil {
		return errors.New("AgentOps task application initializer is required")
	}
	application, err := prepared.Start(ctx)
	if err != nil {
		return err
	}
	if isNilAgentOpsDependency(application) {
		return errors.New("AgentOps task application is required")
	}
	defer r.drainApplication(ctx, task, prepared.Session.ID, application)

	result, err := application.Run(ctx, app.RunCommand{Prompt: prompt}, nil)
	if err != nil {
		return err
	}

	final := "任务执行完成。"
	runID := ""
	tracePath := prepared.TracePath
	metricsPath := prepared.MetricsPath
	if result != nil && result.FinalMessage != "" {
		final = result.FinalMessage
	}
	if result != nil {
		runID = result.RunID
		if result.TracePath != "" {
			tracePath = result.TracePath
		}
		if result.MetricsPath != "" {
			metricsPath = result.MetricsPath
		}
	}

	final += fmt.Sprintf(
		"\n\nSession: %s\nRun: %s\nTrace: %s\nMetrics: %s",
		prepared.Session.ID,
		runID,
		tracePath,
		metricsPath,
	)

	_ = r.deliverTaskText(ctx, task, DeliveryStageFinal, final)
	return nil
}

func (r *Runner) drainApplication(ctx context.Context, task Task, sessionID string, application TaskApplication) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[AgentOps] task=%s session=%s drain panic recovered: %v", task.TaskID, sessionID, recovered)
		}
	}()
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), defaultTerminalSendTimeout)
	defer cancel()
	if err := application.Drain(drainCtx); err != nil {
		log.Printf("[AgentOps] task=%s session=%s drain failed: %v", task.TaskID, sessionID, err)
	}
}

func isNilAgentOpsDependency(value any) bool {
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
