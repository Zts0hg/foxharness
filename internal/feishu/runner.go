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
	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/provider"
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
	locks          map[string]*sync.Mutex
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
		provider:       provider,
		workDir:        workDir,
		messenger:      messenger,
		sessionManager: sessionManager,
		approvalStore:  approvalStore,
		locks:          make(map[string]*sync.Mutex),
	}
}

// Start begins consuming tasks from the tasks channel.  Each task is
// dispatched to a separate goroutine.  Start blocks until the context is
// cancelled or the tasks channel is closed.
func (r *Runner) Start(ctx context.Context, tasks <-chan Task) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-tasks:
			if !ok {
				return
			}
			go r.runOne(ctx, task)
		}
	}
}

func (r *Runner) runOne(ctx context.Context, task Task) {
	runCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	_ = r.messenger.SendText(runCtx, task.ChatID, fmt.Sprintf("已收到任务 %s，开始执行。", task.TaskID))

	sessionKey := task.ChatID + ":" + task.SenderID
	lock := r.lockFor(sessionKey)
	lock.Lock()
	defer lock.Unlock()

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

	registry := r.buildRegistry(sess, task.ChatID)

	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())
	eng := engine.NewAgentEngine(
		r.provider,
		registry,
		r.workDir,
		composer,
		engine.Config{
			EnableThinking: false,
			MaxTurns:       20,
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
	taskPrompt := fmt.Sprintf(
		"以下任务来自飞书用户 %s，消息 ID 为 %s。\n\n%s",
		task.SenderID,
		task.MessageID,
		taskText,
	)

	reporter := NewReporter(r.messenger, task.ChatID, task.TaskID)
	result, err := eng.RunWithReporter(runCtx, sess, taskPrompt, reporter)
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

func (r *Runner) buildRegistry(sess *session.Session, chatID string) tools.Registry {
	approver := approval.NewFeishuApprover(chatID, r.messenger, r.approvalStore)
	subManager := subagent.NewManager(r.provider, r.workDir)

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewReadTodoTool(sess.RootDir))
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	registry.Register(subagent.NewTool(subManager, sess.ID))
	registry.Use(middleware.NewDangerMiddleware(approver))
	return registry
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

func (r *Runner) lockFor(key string) *sync.Mutex {
	r.locksMu.Lock()
	defer r.locksMu.Unlock()

	lock, ok := r.locks[key]
	if !ok {
		lock = &sync.Mutex{}
		r.locks[key] = lock
	}
	return lock
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
