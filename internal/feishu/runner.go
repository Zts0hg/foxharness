package feishu

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// Runner consumes Task values from a channel and executes each one in a
// dedicated goroutine using the full agent engine stack: session creation,
// tool registration (file I/O, bash, sub-agent), danger-action approval
// middleware, context compaction, and a 5-minute per-task timeout.
type Runner struct {
	provider       provider.LLMProvider
	workDir        string
	messenger      *Messenger
	sessionManager *session.Manager
	approvalStore  *approval.Store
	locksMu        sync.Mutex
	locks          map[string]*sessionLock

	maxConcurrentTasks int
	taskTimeout        time.Duration
	lockTTL            time.Duration
	clock              func() time.Time
	newTaskContext     func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	runTask            func(context.Context, Task)
}

const (
	defaultMaxConcurrentTasks = 4
	defaultTaskTimeout        = 5 * time.Minute
	defaultSessionLockTTL     = 30 * time.Minute
)

type sessionLock struct {
	permit   chan struct{}
	refs     int
	lastUsed time.Time
}

// NewRunner constructs a Runner with the given LLM provider, working
// directory, Feishu messenger for user notifications, session manager, and
// approval store.
func NewRunner(
	provider provider.LLMProvider,
	workDir string,
	messenger *Messenger,
	sessionManager *session.Manager,
	approvalStore *approval.Store,
) *Runner {
	return &Runner{
		provider:           provider,
		workDir:            workDir,
		messenger:          messenger,
		sessionManager:     sessionManager,
		approvalStore:      approvalStore,
		locks:              make(map[string]*sessionLock),
		maxConcurrentTasks: defaultMaxConcurrentTasks,
		taskTimeout:        defaultTaskTimeout,
		lockTTL:            defaultSessionLockTTL,
		clock:              time.Now,
	}
}

// Start begins consuming tasks from the tasks channel.  Each task is
// dispatched to a separate goroutine.  Start blocks until the context is
// cancelled or the tasks channel is closed.
func (r *Runner) Start(ctx context.Context, tasks <-chan Task) {
	permits := make(chan struct{}, r.concurrentTaskLimit())
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-tasks:
			if !ok {
				return
			}
			taskCtx, cancel := r.acceptedTaskContext(ctx)
			select {
			case permits <- struct{}{}:
				if taskCtx.Err() != nil {
					<-permits
					cancel()
					continue
				}
			case <-taskCtx.Done():
				cancel()
				continue
			}
			go func(taskCtx context.Context, cancel context.CancelFunc, task Task) {
				defer func() {
					cancel()
					<-permits
					if rec := recover(); rec != nil {
						log.Printf("[Feishu Runner] task=%s panic recovered: %v", task.TaskID, rec)
					}
				}()
				r.taskRunner()(taskCtx, task)
			}(taskCtx, cancel, task)
		}
	}
}

func (r *Runner) runOne(ctx context.Context, task Task) {
	runCtx, cancel := context.WithTimeout(ctx, r.taskTimeoutOrDefault())
	defer cancel()

	_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("已收到任务 %s，开始执行。", task.TaskID))

	sessionKey := task.ChatID + ":" + task.SenderID
	releaseLock, err := r.acquireSessionLock(runCtx, sessionKey)
	if err != nil {
		log.Printf("[Feishu Runner] task=%s session lock cancelled: %v", task.TaskID, err)
		return
	}
	defer releaseLock()

	forceNew, taskText := parseSessionDirective(task.Text)
	sess, created, err := r.resolveSession(forceNew, task)
	if err != nil {
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("创建 Session 失败: %v", err))
		return
	}

	if created {
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务已进入新 Session: %s", sess.ID))
	} else {
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("继续使用 Session: %s", sess.ID))
	}

	autoStore := automemory.NewStore(r.sessionManager.HomeDir(), r.workDir)
	hooks := automemory.NewPerRunHooks(r.provider, autoStore, r.workDir)
	tracker := hooks.NewTracker()

	taskPrompt := fmt.Sprintf(
		"以下任务来自飞书用户 %s，消息 ID 为 %s。\n\n%s",
		task.SenderID,
		task.MessageID,
		taskText,
	)
	registry := r.buildRegistry(sess, task.ChatID, taskPrompt)

	composer := r.buildComposer(sess, autoStore)
	eng := engine.NewAgentEngine(
		r.provider,
		registry,
		r.workDir,
		composer,
		engine.Config{
			EnableThinking: false,
			MaxTurns:       20,
			OnToolCalled:   hooks.RecordCallback(tracker),
		},
	)
	compCfg := compaction.DefaultCompactionConfig()
	compCfg.SessionDir = sess.RootDir
	compCfg.TranscriptPath = sess.TranscriptPath()
	compactor, err := compaction.NewCompactor(r.provider, compCfg)
	if err != nil {
		log.Printf("[Feishu Runner] 初始化 Compactor 失败: %v", err)
		return
	}
	eng.WithCompactor(compactor)

	reporter := NewReporter(r.messenger, task.ChatID, task.TaskID)
	result, err := eng.RunWithReporter(runCtx, sess, taskPrompt, reporter)
	if result != nil {
		r.fireMemoryExtraction(hooks, sess, result.RunID, tracker)
	}
	if err != nil {
		log.Printf("[Feishu Runner] task=%s session=%s  failed: %v", task.TaskID, sess.ID, err)
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("Session %s 执行失败：%v", sess.ID, err))
		return
	}

	if result == nil || result.FinalMessage == "" {
		_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务 %s 执行完成，Session: %s", task.TaskID, sess.ID))
		return
	}

	_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("任务 %s 已完成，Session: %s，Run: %s", task.TaskID, sess.ID, result.RunID))
}

// buildComposer assembles the system-prompt composer for a task, injecting the
// cross-session persistent memory index when a store is available (REQ-006).
func (r *Runner) buildComposer(sess *session.Session, store *automemory.Store) *prompt.Composer {
	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())
	if store != nil {
		composer = composer.WithAutoMemory(store)
	}
	return composer
}

// fireMemoryExtraction launches the post-run memory extraction hook (PLD-8). It
// is fire-and-forget and panic-guarded so it can never disturb the task result.
func (r *Runner) fireMemoryExtraction(hooks *automemory.PerRunHooks, sess *session.Session, runID string, tracker *automemory.Tracker) {
	if hooks == nil {
		return
	}
	defer func() {
		if rec := recover(); rec != nil {
			log.Printf("[Feishu Runner] memory extraction launch panic recovered: %v", rec)
		}
	}()
	hooks.Fire(sess, runID, tracker)
}

func (r *Runner) buildRegistry(sess *session.Session, chatID string, currentPrompt ...string) tools.Registry {
	evidenceProvider := remotePermissionEvidenceProvider(sess, firstPrompt(currentPrompt))
	var approver permission.UserApprover
	if r.messenger != nil && r.approvalStore != nil {
		approver = approval.NewPermissionApprover(chatID, r.messenger, r.approvalStore)
	}
	coordinator := permission.NewCoordinator(permission.Config{
		State:     permission.NewState(permission.ModeAsk, false),
		Workspace: r.workDir,
		CWD:       r.workDir,
		Source:    permission.SourceMain,
		Approver:  approver,
		Evidence:  evidenceProvider,
	})
	subManager := subagent.NewManager(r.provider, r.workDir).
		WithPermission(coordinator).
		WithParentEvidence(evidenceProvider)

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewReadTodoTool(sess.RootDir))
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	registry.Register(subagent.NewTool(subManager, sess.ID))
	return permission.DecorateRegistry(registry, coordinator, evidenceProvider)
}

func remotePermissionEvidenceProvider(sess *session.Session, currentPrompt string) permission.EvidenceProvider {
	return func(request permission.Request) permission.Evidence {
		var messages []schema.Message
		if sess != nil {
			records, err := session.NewMessageLog(sess).LoadRecords()
			if err == nil {
				messages = make([]schema.Message, 0, len(records)+1)
				for _, record := range records {
					messages = append(messages, record.Message)
				}
			}
		}
		if strings.TrimSpace(currentPrompt) != "" && !containsDirectUserMessage(messages, currentPrompt) {
			messages = append(messages, schema.Message{Role: schema.RoleUser, Content: currentPrompt})
		}
		return permission.BuildEvidence(messages, nil, request)
	}
}

func firstPrompt(prompts []string) string {
	if len(prompts) == 0 {
		return ""
	}
	return prompts[0]
}

func containsDirectUserMessage(messages []schema.Message, content string) bool {
	for _, message := range messages {
		if message.Role == schema.RoleUser && message.ToolCallID == "" && message.Content == content {
			return true
		}
	}
	return false
}

func (r *Runner) resolveSession(forceNew bool, task Task) (*session.Session, bool, error) {
	if !forceNew {
		sess, err := r.sessionManager.Latest(session.LookupOptions{
			Source: session.SOURCEFeishu,
			UserID: task.SenderID,
			ChatID: task.ChatID,
		})
		if err == nil {
			return sess, false, nil
		}
		if !errors.Is(err, session.ErrNotFound) {
			return nil, false, err
		}
	}

	sess, err := r.sessionManager.Create(session.CreateOptions{
		Source:  session.SOURCEFeishu,
		WorkDir: r.workDir,
		UserID:  task.SenderID,
		ChatID:  task.ChatID,
	})
	if err != nil {
		return nil, false, err
	}
	return sess, true, nil
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
