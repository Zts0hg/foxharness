package feishu

import (
	"errors"
	"log"
	"strings"
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

type legacyChildRunnerConfig struct {
	Provider       provider.LLMProvider
	WorkDir        string
	Permission     *permission.Coordinator
	ParentEvidence permission.EvidenceProvider
}

type legacyFeishuRunner struct {
	provider       provider.LLMProvider
	workDir        string
	messenger      TextMessenger
	sessionManager *session.FileStore
	approvalStore  *approval.Store
	newChildRunner func(legacyChildRunnerConfig) subagent.Runner
}

func (*legacyFeishuRunner) taskTimeoutOrDefault() time.Duration { return defaultTaskTimeout }

func (*legacyFeishuRunner) concurrentTaskLimit() int { return defaultMaxConcurrentTasks }

func newLegacyFeishuRunner(modelProvider provider.LLMProvider, workDir string, messenger TextMessenger, manager *session.FileStore, approvals *approval.Store) *legacyFeishuRunner {
	return &legacyFeishuRunner{
		provider: modelProvider, workDir: workDir, messenger: messenger,
		sessionManager: manager, approvalStore: approvals,
	}
}

func (r *legacyFeishuRunner) buildComposer(sess *session.StoredSession, store *automemory.Store) *prompt.Composer {
	composer := prompt.NewComposer(r.workDir).WithMemory(sess.MemoryPath())
	if store != nil {
		composer = composer.WithAutoMemory(store)
	}
	return composer
}

func (r *legacyFeishuRunner) fireMemoryExtraction(hooks *automemory.PerRunHooks, sess *session.StoredSession, runID string, tracker *automemory.Tracker) {
	if hooks == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[Feishu legacy test adapter] memory extraction launch panic recovered: %v", recovered)
		}
	}()
	hooks.Fire(sess, runID, tracker)
}

func (r *legacyFeishuRunner) buildRegistry(sess *session.StoredSession, chatID string, currentPrompt ...string) tools.Registry {
	evidenceProvider := legacyRemotePermissionEvidenceProvider(sess, firstLegacyPrompt(currentPrompt))
	var approver permission.UserApprover
	if r.messenger != nil && r.approvalStore != nil {
		approver = approval.NewPermissionApprover(chatID, r.messenger, r.approvalStore)
	}
	coordinator := permission.NewCoordinator(permission.Config{
		State: permission.NewState(permission.ModeAsk, false), Workspace: r.workDir, CWD: r.workDir,
		Source: permission.SourceMain, Approver: approver, Evidence: evidenceProvider,
	})
	var childRunner subagent.Runner
	if r.newChildRunner != nil {
		childRunner = r.newChildRunner(legacyChildRunnerConfig{
			Provider: r.provider, WorkDir: r.workDir, Permission: coordinator, ParentEvidence: evidenceProvider,
		})
	}
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(r.workDir))
	registry.Register(tools.NewWriteFileTool(r.workDir))
	registry.Register(tools.NewEditFileTool(r.workDir))
	registry.Register(tools.NewBashTool(r.workDir))
	registry.Register(tools.NewReadTodoTool(sess.RootDir))
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	registry.Register(subagent.NewTool(childRunner, string(sess.ID)))
	return permission.DecorateRegistry(registry, coordinator, evidenceProvider)
}

func legacyRemotePermissionEvidenceProvider(sess *session.StoredSession, currentPrompt string) permission.EvidenceProvider {
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
		if strings.TrimSpace(currentPrompt) != "" && !legacyContainsDirectUserMessage(messages, currentPrompt) {
			messages = append(messages, schema.Message{Role: schema.RoleUser, Content: currentPrompt})
		}
		return permission.BuildEvidence(messages, nil, request)
	}
}

func firstLegacyPrompt(prompts []string) string {
	if len(prompts) == 0 {
		return ""
	}
	return prompts[0]
}

func legacyContainsDirectUserMessage(messages []schema.Message, content string) bool {
	for _, message := range messages {
		if message.Role == schema.RoleUser && message.ToolCallID == "" && message.Content == content {
			return true
		}
	}
	return false
}

func (r *legacyFeishuRunner) resolveSession(forceNew bool, task Task) (*session.StoredSession, bool, error) {
	if !forceNew {
		sess, err := r.sessionManager.Latest(session.LookupOptions{Source: session.SOURCEFeishu, UserID: task.SenderID, ChatID: task.ChatID})
		if err == nil {
			return sess, false, nil
		}
		if !errors.Is(err, session.ErrNotFound) {
			return nil, false, err
		}
	}
	sess, err := r.sessionManager.Create(session.CreateOptions{
		Source: session.SOURCEFeishu, WorkDir: r.workDir, UserID: task.SenderID, ChatID: task.ChatID,
	})
	return sess, true, err
}

type taskProviderMetadata struct {
	protocol string
	model    string
}

type taskProviderMetadataSource interface {
	ProviderProtocol() string
	ModelName() string
}

func snapshotTaskProviderMetadata(modelProvider provider.LLMProvider) taskProviderMetadata {
	metadata, ok := modelProvider.(taskProviderMetadataSource)
	if !ok {
		return taskProviderMetadata{}
	}
	return taskProviderMetadata{protocol: metadata.ProviderProtocol(), model: metadata.ModelName()}
}

func (m taskProviderMetadata) apply(engineConfig *engine.Config, compactionConfig *compaction.CompactionConfig) {
	engineConfig.ProviderProtocol = m.protocol
	engineConfig.Model = m.model
	compactionConfig.Model = m.model
}
