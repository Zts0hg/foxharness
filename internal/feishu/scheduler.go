package feishu

import (
	"context"
	"log"
)

type scheduledTask struct {
	task   Task
	ctx    context.Context
	cancel context.CancelFunc
}

type sessionTaskQueue struct {
	tasks  []scheduledTask
	active bool
	ready  bool
}

type taskScheduler struct {
	runner    *Runner
	limit     int
	active    int
	sessions  map[string]*sessionTaskQueue
	ready     []string
	completed chan string
}

func newTaskScheduler(runner *Runner) *taskScheduler {
	limit := runner.concurrentTaskLimit()
	return &taskScheduler{
		runner:    runner,
		limit:     limit,
		sessions:  make(map[string]*sessionTaskQueue),
		completed: make(chan string, limit),
	}
}

func (s *taskScheduler) enqueue(parent context.Context, task Task) {
	taskCtx, cancel := s.runner.acceptedTaskContext(parent)
	key := taskSessionKey(task)
	queue, ok := s.sessions[key]
	if !ok {
		queue = &sessionTaskQueue{}
		s.sessions[key] = queue
	}
	queue.tasks = append(queue.tasks, scheduledTask{task: task, ctx: taskCtx, cancel: cancel})
	s.markReady(key, queue)
}

func (s *taskScheduler) dispatch() {
	for s.active < s.limit && len(s.ready) > 0 {
		key := s.ready[0]
		s.ready = s.ready[1:]
		queue := s.sessions[key]
		if queue == nil {
			continue
		}
		queue.ready = false
		if queue.active {
			continue
		}

		task, ok := s.nextRunnable(queue)
		if !ok {
			delete(s.sessions, key)
			continue
		}
		queue.active = true
		s.active++
		go s.run(key, task)
	}
}

func (s *taskScheduler) nextRunnable(queue *sessionTaskQueue) (scheduledTask, bool) {
	for len(queue.tasks) > 0 {
		task := queue.tasks[0]
		queue.tasks = queue.tasks[1:]
		if task.ctx.Err() == nil {
			return task, true
		}
		log.Printf("[Feishu Runner] task=%s expired before execution: %v", task.task.TaskID, task.ctx.Err())
		task.cancel()
	}
	return scheduledTask{}, false
}

func (s *taskScheduler) run(key string, task scheduledTask) {
	defer func() {
		task.cancel()
		if rec := recover(); rec != nil {
			log.Printf("[Feishu Runner] task=%s panic recovered: %v", task.task.TaskID, rec)
		}
		s.completed <- key
	}()
	s.runner.taskRunner()(task.ctx, task.task)
}

func (s *taskScheduler) complete(key string) {
	queue := s.sessions[key]
	if queue == nil || !queue.active {
		return
	}
	queue.active = false
	s.active--
	if len(queue.tasks) == 0 {
		delete(s.sessions, key)
		return
	}
	s.markReady(key, queue)
}

func (s *taskScheduler) markReady(key string, queue *sessionTaskQueue) {
	if queue.active || queue.ready || len(queue.tasks) == 0 {
		return
	}
	queue.ready = true
	s.ready = append(s.ready, key)
}

func taskSessionKey(task Task) string {
	return task.ChatID + ":" + task.SenderID
}
