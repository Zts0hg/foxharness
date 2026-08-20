package agentops

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/approval"
	"github.com/Zts0hg/foxharness/internal/automemory"
	"github.com/Zts0hg/foxharness/internal/compaction"
	legacycontext "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type legacyAgentOpsRunner struct {
	provider       provider.LLMProvider
	workDir        string
	logDir         string
	messenger      Messenger
	sessions       *session.FileStore
	approvalStore  *approval.Store
	newChildRunner legacyChildRunnerFactory
}

type legacyChildRunnerConfig struct {
	Provider       provider.LLMProvider
	WorkDir        string
	Permission     *permission.Coordinator
	ParentEvidence permission.EvidenceProvider
}

type legacyChildRunnerFactory func(legacyChildRunnerConfig) subagent.Runner

func newLegacyAgentOpsRunner(
	modelProvider provider.LLMProvider,
	workDir string,
	logDir string,
	messenger Messenger,
	approvals *approval.Store,
) *legacyAgentOpsRunner {
	return &legacyAgentOpsRunner{
		provider: modelProvider, workDir: workDir, logDir: logDir,
		messenger: messenger, sessions: session.NewFileStore(workDir), approvalStore: approvals,
	}
}

func (*legacyAgentOpsRunner) concurrentTaskLimit() int            { return defaultMaxConcurrentTasks }
func (*legacyAgentOpsRunner) taskTimeoutOrDefault() time.Duration { return defaultTaskTimeout }

func (r *legacyAgentOpsRunner) run(ctx context.Context, task Task) error {
	taskProvider := snapshotTaskProvider(r.provider)
	stored, err := r.sessions.Create(session.CreateOptions{
		Source: session.SOURCEFeishu, WorkDir: r.workDir,
		UserID: task.SenderID, ChatID: task.ChatID,
	})
	if err != nil {
		return err
	}
	if r.messenger != nil {
		_ = r.messenger.SendText(ctx, task.ChatID, fmt.Sprintf("已创建 AgentOps Session: %s\n开始分析。", stored.ID))
	}
	workingMemory := memory.NewSessionStore(r.workDir, stored.RootDir)
	if err := workingMemory.EnsureFiles(); err != nil {
		return err
	}
	taskPrompt := BuildPrompt(task)
	autoMemory := automemory.NewStore(r.sessions.HomeDir(), r.workDir)
	hooks := automemory.NewPerRunHooks(taskProvider.provider, autoMemory, r.workDir)
	tracker := hooks.NewTracker()
	registry := r.buildRegistry(task, stored, taskProvider.provider)
	composer := r.buildComposer(stored, autoMemory)
	engineConfig := engine.Config{MaxTurns: 24, OnToolCalled: hooks.RecordCallback(tracker)}
	compactionConfig := compaction.DefaultCompactionConfig()
	taskProvider.apply(&engineConfig, &compactionConfig)
	legacyEngine := engine.NewLegacyEngine(taskProvider.provider, registry, r.workDir, composer, engineConfig)
	compactionConfig.SessionDir = stored.RootDir
	compactionConfig.TranscriptPath = stored.TranscriptPath()
	compactor, err := compaction.NewCompactor(taskProvider.provider, compactionConfig)
	if err != nil {
		return fmt.Errorf("初始化 Compactor 失败: %w", err)
	}
	legacyEngine.WithCompactor(compactor)
	result, err := legacyEngine.Run(ctx, stored, taskPrompt)
	if result != nil {
		r.fireMemoryExtraction(hooks, stored, result.RunID, tracker)
	}
	if err != nil {
		return err
	}
	final := "任务执行完成。"
	runID := ""
	tracePath := stored.TracePath()
	metricsPath := stored.MetricsPath()
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
	final += fmt.Sprintf("\n\nSession: %s\nRun: %s\nTrace: %s\nMetrics: %s", stored.ID, runID, tracePath, metricsPath)
	if r.messenger == nil {
		return errMessengerUnavailable
	}
	return r.messenger.SendText(ctx, task.ChatID, truncateAgentOpsText(final))
}

func (r *legacyAgentOpsRunner) buildComposer(stored *session.StoredSession, store *automemory.Store) *legacycontext.Composer {
	workingMemory := memory.NewSessionStore(r.workDir, stored.RootDir)
	composer := legacycontext.NewComposer(r.workDir).WithMemory(workingMemory.WorkingMemoryPath())
	if store != nil {
		composer = composer.WithAutoMemory(store)
	}
	return composer
}

func (r *legacyAgentOpsRunner) fireMemoryExtraction(hooks *automemory.PerRunHooks, stored *session.StoredSession, runID string, tracker *automemory.Tracker) {
	if hooks == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[AgentOps] memory extraction launch panic recovered: %v", recovered)
		}
	}()
	hooks.Fire(stored, runID, tracker)
}

func (r *legacyAgentOpsRunner) buildRegistry(task Task, stored *session.StoredSession, taskProviders ...provider.LLMProvider) tools.Registry {
	registry := tools.NewRegistry()
	evidence := agentOpsPermissionEvidenceProvider(stored, BuildPrompt(task))
	var approver permission.UserApprover
	if r.messenger != nil && r.approvalStore != nil {
		approver = approval.NewPermissionApprover(task.ChatID, r.messenger, r.approvalStore)
	}
	coordinator := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeAsk, false), Workspace: r.workDir, CWD: r.workDir,
		Source: permission.SourceMain, Approver: approver, Evidence: evidence,
	})
	registry.Register(NewLogSearchTool(r.logDir))
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))
	registry.Register(tools.NewReadTodoTool(stored.RootDir))
	registry.Register(tools.NewUpdateTodoTool(stored.RootDir))
	taskProvider := r.provider
	if len(taskProviders) > 0 && taskProviders[0] != nil {
		taskProvider = taskProviders[0]
	}
	var child subagent.Runner
	if r.newChildRunner != nil {
		child = r.newChildRunner(legacyChildRunnerConfig{
			Provider: taskProvider, WorkDir: r.workDir,
			Permission: coordinator, ParentEvidence: evidence,
		})
	}
	registry.Register(subagent.NewTool(child, string(stored.ID)))
	return permission.DecorateRegistry(registry, coordinator, evidence)
}

func agentOpsPermissionEvidenceProvider(stored *session.StoredSession, currentPrompt string) permission.EvidenceProvider {
	return func(request permission.Request) permission.Evidence {
		var messages []schema.Message
		if stored != nil {
			records, err := session.NewMessageLog(stored).LoadRecords()
			if err == nil {
				messages = make([]schema.Message, 0, len(records)+1)
				for _, record := range records {
					messages = append(messages, record.Message)
				}
			}
		}
		if strings.TrimSpace(currentPrompt) != "" && !agentOpsContainsDirectUserMessage(messages, currentPrompt) {
			messages = append(messages, schema.Message{Role: schema.RoleUser, Content: currentPrompt})
		}
		return permission.BuildEvidence(messages, nil, request)
	}
}

func agentOpsContainsDirectUserMessage(messages []schema.Message, content string) bool {
	for _, message := range messages {
		if message.Role == schema.RoleUser && message.ToolCallID == "" && message.Content == content {
			return true
		}
	}
	return false
}

type taskProviderMetadataSource interface {
	ProviderProtocol() string
	ModelName() string
}

type taskProviderSnapshot struct {
	provider taskScopedProvider
}

type taskScopedProvider struct {
	provider.LLMProvider
	protocol string
	model    string
}

func snapshotTaskProvider(modelProvider provider.LLMProvider) taskProviderSnapshot {
	snapshot := taskProviderSnapshot{provider: taskScopedProvider{LLMProvider: modelProvider}}
	metadata, ok := modelProvider.(taskProviderMetadataSource)
	if !ok {
		return snapshot
	}
	snapshot.provider.protocol = metadata.ProviderProtocol()
	snapshot.provider.model = metadata.ModelName()
	return snapshot
}

func (p taskScopedProvider) ProviderProtocol() string { return p.protocol }
func (p taskScopedProvider) ModelName() string        { return p.model }

func (s taskProviderSnapshot) apply(engineConfig *engine.Config, compactionConfig *compaction.CompactionConfig) {
	engineConfig.ProviderProtocol = s.provider.protocol
	engineConfig.Model = s.provider.model
	compactionConfig.Model = s.provider.model
}
