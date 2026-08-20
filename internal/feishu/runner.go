package feishu

import (
	"context"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
)

/* Runner consumes tasks while retaining Feishu scheduling and presentation ownership. */
type Runner struct {
	executionFactory TaskExecutionFactory
	messenger        TextMessenger
	locksMu          sync.Mutex
	locks            map[string]*sessionLock

	maxConcurrentTasks      int
	taskTimeout             time.Duration
	lockTTL                 time.Duration
	clock                   func() time.Time
	newTaskContext          func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	runTask                 func(context.Context, Task)
	taskOutcomeObserver     TaskOutcomeObserver
	deliveryFailureObserver DeliveryFailureObserver
}

const (
	defaultMaxConcurrentTasks  = 4
	defaultTaskTimeout         = 5 * time.Minute
	defaultSessionLockTTL      = 30 * time.Minute
	defaultTerminalSendTimeout = 10 * time.Second
)

type sessionLock struct {
	permit   chan struct{}
	refs     int
	lastUsed time.Time
}

/* NewRunner constructs a Feishu control-plane runner over an application execution factory. */
func NewRunner(executionFactory TaskExecutionFactory, messenger TextMessenger) *Runner {
	return &Runner{
		executionFactory:    executionFactory,
		messenger:           messenger,
		locks:               make(map[string]*sessionLock),
		maxConcurrentTasks:  defaultMaxConcurrentTasks,
		taskTimeout:         defaultTaskTimeout,
		lockTTL:             defaultSessionLockTTL,
		clock:               time.Now,
		taskOutcomeObserver: loggingTaskOutcomeObserver{},
	}
}

/* WithDeliveryFailureObserver installs the non-blocking delivery failure observer. */
func (r *Runner) WithDeliveryFailureObserver(observer DeliveryFailureObserver) *Runner {
	r.deliveryFailureObserver = observer
	return r
}

// Start begins consuming tasks from the tasks channel.  Each task is
// dispatched to a separate goroutine.  Start blocks until the context is
// cancelled or the tasks channel is closed.
func (r *Runner) Start(ctx context.Context, tasks <-chan Task) {
	scheduler := newTaskScheduler(r)
	taskInput := tasks
	cancellation := ctx.Done()
	cancelling := false
	for {
		if cancellation != nil && ctx.Err() != nil {
			scheduler.cancelAll(ctx.Err())
			taskInput = nil
			cancellation = nil
			cancelling = true
		}
		if !cancelling {
			scheduler.dispatch()
		}
		if taskInput == nil && scheduler.idle() {
			return
		}
		select {
		case <-cancellation:
			scheduler.cancelAll(ctx.Err())
			taskInput = nil
			cancellation = nil
			cancelling = true
		case task, ok := <-taskInput:
			if !ok {
				taskInput = nil
				continue
			}
			scheduler.enqueue(ctx, task)
			if ctx.Err() != nil {
				scheduler.cancelAll(ctx.Err())
				taskInput = nil
				cancellation = nil
				cancelling = true
			}
		case sessionKey := <-scheduler.completed:
			scheduler.complete(sessionKey)
		}
	}
}

func (r *Runner) runOne(ctx context.Context, task Task) {
	runCtx, cancel := context.WithTimeout(ctx, r.taskTimeoutOrDefault())
	defer cancel()

	r.deliverTaskText(runCtx, task, DeliveryStageReceipt, fmt.Sprintf("已收到任务 %s，开始执行。", task.TaskID))

	sessionKey := taskSessionKey(task)
	releaseLock, err := r.acquireSessionLock(runCtx, sessionKey)
	if err != nil {
		log.Printf("[Feishu Runner] task=%s session lock cancelled: %v", task.TaskID, err)
		r.deliverCancellation(task, err)
		return
	}
	defer releaseLock()

	forceNew, taskText := parseSessionDirective(task.Text)
	taskPrompt := feishuTaskPrompt(task, taskText)
	prepared, err := r.prepareTask(runCtx, TaskExecutionRequest{
		Task: task, Prompt: taskPrompt, ForceNewSession: forceNew,
	})
	if err != nil {
		r.deliverTaskText(runCtx, task, DeliveryStageSession, fmt.Sprintf("创建 Session 失败: %v", err))
		return
	}
	if prepared.Drain != nil {
		defer func() {
			drainCtx, cancelDrain := context.WithTimeout(context.WithoutCancel(runCtx), defaultTerminalSendTimeout)
			defer cancelDrain()
			if drainErr := prepared.Drain(drainCtx); drainErr != nil {
				log.Printf("[Feishu Runner] task=%s session=%s drain failed: %v", task.TaskID, prepared.Session.ID, drainErr)
			}
		}()
	}

	if prepared.Created {
		r.deliverTaskText(runCtx, task, DeliveryStageSession, fmt.Sprintf("任务已进入新 Session: %s", prepared.Session.ID))
	} else {
		r.deliverTaskText(runCtx, task, DeliveryStageSession, fmt.Sprintf("继续使用 Session: %s", prepared.Session.ID))
	}
	if prepared.SetupError != nil {
		log.Printf("[Feishu Runner] task=%s session=%s runtime setup failed: %v", task.TaskID, prepared.Session.ID, prepared.SetupError)
		r.deliverTaskText(runCtx, task, DeliveryStageFailure, fmt.Sprintf("初始化任务执行环境失败：%v", prepared.SetupError))
		return
	}

	reporter := NewReporter(r.messenger, task.ChatID, task.TaskID).WithDeliveryFailureObserver(r.deliveryFailureObserver)
	result, err := prepared.Application.Run(runCtx, app.RunCommand{Prompt: taskPrompt}, reporter)
	if err != nil {
		log.Printf("[Feishu Runner] task=%s session=%s failed: %v", task.TaskID, prepared.Session.ID, err)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			r.deliverCancellation(task, err)
		} else {
			r.deliverTaskText(runCtx, task, DeliveryStageFailure, fmt.Sprintf("Session %s 执行失败：%v", prepared.Session.ID, err))
		}
		return
	}

	if result == nil || result.FinalMessage == "" {
		r.deliverTaskText(runCtx, task, DeliveryStageFinal, fmt.Sprintf("任务 %s 执行完成，Session: %s", task.TaskID, prepared.Session.ID))
		return
	}

	r.deliverTaskText(runCtx, task, DeliveryStageFinal, fmt.Sprintf("任务 %s 已完成，Session: %s，Run: %s", task.TaskID, prepared.Session.ID, result.RunID))
}

func (r *Runner) prepareTask(ctx context.Context, request TaskExecutionRequest) (PreparedTaskExecution, error) {
	if isNilDependency(r.executionFactory) {
		return PreparedTaskExecution{}, errors.New("Feishu task execution factory is required")
	}
	prepared, err := r.executionFactory.PrepareTask(ctx, request)
	if err != nil {
		return PreparedTaskExecution{}, err
	}
	if isNilDependency(prepared.Application) && prepared.SetupError == nil {
		return PreparedTaskExecution{}, errors.New("Feishu task application is required")
	}
	if strings.TrimSpace(prepared.Session.ID) == "" {
		return PreparedTaskExecution{}, errors.New("Feishu task session identity is required")
	}
	return prepared, nil
}

func feishuTaskPrompt(task Task, taskText string) string {
	return fmt.Sprintf(
		"以下任务来自飞书用户 %s，消息 ID 为 %s。\n\n%s",
		task.SenderID,
		task.MessageID,
		taskText,
	)
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

func (r *Runner) lockTTLOrDefault() time.Duration {
	if r.lockTTL > 0 {
		return r.lockTTL
	}
	return defaultSessionLockTTL
}

func (r *Runner) now() time.Time {
	if r.clock != nil {
		return r.clock()
	}
	return time.Now()
}

func (r *Runner) taskRunner() func(context.Context, Task) {
	if r.runTask != nil {
		return r.runTask
	}
	return r.runOne
}

func (r *Runner) acceptedTaskContext(parent context.Context) (context.Context, context.CancelFunc) {
	if r.newTaskContext != nil {
		return r.newTaskContext(parent, r.taskTimeoutOrDefault())
	}
	return context.WithTimeout(parent, r.taskTimeoutOrDefault())
}

func (r *Runner) handleTaskPanic(task Task, recovered any) {
	cause := fmt.Sprintf("panic recovered: %v", recovered)
	r.observeTaskOutcome(TaskOutcome{
		TaskID: task.TaskID,
		ChatID: task.ChatID,
		Status: TaskOutcomeFailed,
		Error:  cause,
	})
	deliveryCtx, cancel := context.WithTimeout(context.Background(), defaultTerminalSendTimeout)
	defer cancel()
	message := fmt.Sprintf("任务 %s 执行失败：内部错误。", task.TaskID)
	r.deliverTaskText(deliveryCtx, task, DeliveryStagePanicFailure, message)
}

func (r *Runner) deliverTaskText(ctx context.Context, task Task, stage DeliveryStage, text string) {
	if isNilDependency(r.messenger) {
		r.observeDeliveryFailure(DeliveryFailure{TaskID: task.TaskID, ChatID: task.ChatID, Stage: stage, Cause: errMessengerUnavailable})
		return
	}
	if err := r.messenger.SendText(ctx, task.ChatID, text); err != nil {
		r.observeDeliveryFailure(DeliveryFailure{TaskID: task.TaskID, ChatID: task.ChatID, Stage: stage, Cause: err})
	}
}

func isNilDependency(value any) bool {
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

func (r *Runner) deliverCancellation(task Task, cause error) {
	deliveryCtx, cancel := context.WithTimeout(context.Background(), defaultTerminalSendTimeout)
	defer cancel()
	r.deliverTaskText(deliveryCtx, task, DeliveryStageCancellation, fmt.Sprintf("任务 %s 已取消：%v", task.TaskID, cause))
}

func (r *Runner) observeDeliveryFailure(failure DeliveryFailure) {
	if r.deliveryFailureObserver == nil {
		log.Printf("[Feishu Delivery] task=%s chat=%s stage=%s failed: %v", failure.TaskID, failure.ChatID, failure.Stage, failure.Cause)
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[Feishu Runner] task=%s delivery observer panic recovered: %v", failure.TaskID, recovered)
		}
	}()
	r.deliveryFailureObserver.ObserveDeliveryFailure(failure)
}

func (r *Runner) observeTaskOutcome(outcome TaskOutcome) {
	if r.taskOutcomeObserver == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[Feishu Runner] task=%s outcome observer panic recovered: %v", outcome.TaskID, recovered)
		}
	}()
	r.taskOutcomeObserver.ObserveTaskOutcome(outcome)
}

func (r *Runner) acquireSessionLock(ctx context.Context, key string) (func(), error) {
	r.locksMu.Lock()
	if r.locks == nil {
		r.locks = make(map[string]*sessionLock)
	}
	now := r.now()
	r.cleanupSessionLocksLocked(now)
	lock, ok := r.locks[key]
	if !ok {
		lock = &sessionLock{
			permit:   make(chan struct{}, 1),
			lastUsed: now,
		}
		lock.permit <- struct{}{}
		r.locks[key] = lock
	}
	lock.refs++
	r.locksMu.Unlock()

	select {
	case <-lock.permit:
		var once sync.Once
		return func() {
			once.Do(func() {
				lock.permit <- struct{}{}
				r.releaseSessionLockReference(lock)
			})
		}, nil
	case <-ctx.Done():
		r.releaseSessionLockReference(lock)
		return nil, ctx.Err()
	}
}

func (r *Runner) releaseSessionLockReference(lock *sessionLock) {
	r.locksMu.Lock()
	defer r.locksMu.Unlock()
	lock.refs--
	now := r.now()
	lock.lastUsed = now
	r.cleanupSessionLocksLocked(now)
}

func (r *Runner) cleanupSessionLocks() {
	r.locksMu.Lock()
	defer r.locksMu.Unlock()
	r.cleanupSessionLocksLocked(r.now())
}

func (r *Runner) cleanupSessionLocksLocked(now time.Time) {
	for key, lock := range r.locks {
		if lock.refs == 0 && now.Sub(lock.lastUsed) > r.lockTTLOrDefault() {
			delete(r.locks, key)
		}
	}
}

func parseSessionDirective(text string) (bool, string) {
	trimmed := strings.TrimSpace(text)
	for _, prefix := range []string{"/new", "新会话"} {
		if trimmed == prefix {
			return true, trimmed
		}
		if strings.HasPrefix(trimmed, prefix+" ") {
			return true, strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return false, trimmed
}
