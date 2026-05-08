package engine

import (
	"context"
	"fmt"
	"log"
	"time"

	"sync"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/metrics"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/recovery"
	"github.com/Zts0hg/foxharness/internal/reminder"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type PromptComposer interface {
	Compose(userPrompt string) (string, error)
}

type RunResult struct {
	FinalMessage string
	SessionID    string
}

type AgentEngine struct {
	provider       provider.LLMProvider
	registry       tools.Registry
	workDir        string
	enableThinking bool
	composer       PromptComposer
	compactor      *compaction.Compactor
	recovery       *recovery.Tracker
	reminder       *reminder.Manager
}

type indexedToolResult struct {
	Index      int
	Call       schema.ToolCall
	Result     schema.ToolResult
	DurationMS int64
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string, enableThinking bool, composer PromptComposer) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		workDir:        workDir,
		enableThinking: enableThinking,
		composer:       composer,
		recovery:       recovery.NewTracker(),
		reminder:       reminder.NewManager(),
	}
}

func (e *AgentEngine) WithCompactor(c *compaction.Compactor) {
	e.compactor = c
}

func (e *AgentEngine) executeToolCalls(ctx context.Context, calls []schema.ToolCall) []indexedToolResult {
	results := make([]indexedToolResult, len(calls))

	flushParallelBatch := func(batch []int) {
		if len(batch) == 0 {
			return
		}
		var wg sync.WaitGroup
		wg.Add(len(batch))

		for _, idx := range batch {
			idx := idx
			go func() {
				defer wg.Done()

				call := calls[idx]
				started := time.Now()
				result := e.registry.Execute(ctx, call)
				durationMS := time.Since(started).Milliseconds()
				results[idx] = indexedToolResult{
					Index:      idx,
					Call:       call,
					Result:     result,
					DurationMS: durationMS,
				}
			}()
		}
		wg.Wait()
	}

	var parallelBatch []int
	for i, call := range calls {
		if e.registry.IsParallelSafe(call.Name) {
			parallelBatch = append(parallelBatch, i)
			continue
		}

		flushParallelBatch(parallelBatch)
		parallelBatch = nil

		started := time.Now()
		result := e.registry.Execute(ctx, call)
		durationMS := time.Since(started).Milliseconds()
		results[i] = indexedToolResult{
			Index:      i,
			Call:       call,
			Result:     result,
			DurationMS: durationMS,
		}
	}

	flushParallelBatch(parallelBatch)
	return results
}

func (e *AgentEngine) Run(ctx context.Context, sess *session.Session, userPrompt string) (*RunResult, error) {
	log.Printf("[Engine] 引擎启动，Session: %s，WorkDir: %s\n", sess.ID, e.workDir)
	log.Printf("[Engine] 慢思考模式（Thinking Phase）: %v\n", e.enableThinking)

	recorder := metrics.NewRecorder(sess.MetricsPath())
	aggregator := metrics.NewAggregator()
	estimator := metrics.RoughEstimator{}

	summaryWritten := false
	defer func() {
		if !summaryWritten {
			_ = recorder.Append(aggregator.Summary(sess.ID))
		}
	}()

	transcript := session.NewTranscript(sess)
	_ = transcript.Append("user_prompt", map[string]string{"prompt": userPrompt})

	systemPrompt, err := e.composer.Compose(userPrompt)
	if err != nil {
		return nil, fmt.Errorf("组装系统提示词失败: %w", err)
	}

	contextHistory := []schema.Message{
		{
			Role:    schema.RoleSystem,
			Content: systemPrompt,
		},
		{
			Role:    schema.RoleUser,
			Content: userPrompt,
		},
	}

	turnCount := 0
	final := ""

	for {
		turnCount++
		log.Printf("====== [Turn %d] 开始", turnCount)

		if e.compactor != nil {
			compacted, err := e.compactor.MaybeCompact(ctx, contextHistory)
			if err != nil {
				log.Printf("[Compactor] 压缩失败，将继续使用原始上下文: %v", err)
			} else if len(compacted) != len(contextHistory) {
				log.Printf("[Compactor] 上下文已压缩: %d -> %d 条消息", len(contextHistory), len(compacted))
				contextHistory = compacted
				_ = transcript.Append("context_compacted", map[string]any{
					"turn": turnCount,
				})
			}
		}
		if e.recovery.ShouldInject() {
			recoveryPrompt := e.recovery.BuildPrompt()
			if recoveryPrompt != "" {
				contextHistory = append(contextHistory, schema.Message{
					Role:    schema.RoleUser,
					Content: "[Runtime System Notice]\n\n" + recoveryPrompt,
				})

				_ = transcript.Append("error_recovery_injected", map[string]any{
					"prompt": recoveryPrompt,
				})
				log.Printf("[Recovery] 已注入错误恢复提示")
				e.recovery.MarkInject()
			}
		}

		if msg, ok := e.reminder.MaybeBuild(turnCount); ok {
			contextHistory = append(contextHistory, schema.Message{
				Role:    schema.RoleUser,
				Content: "[Runtime System Reminder]\n\n" + msg,
			})

			_ = transcript.Append("system_reminder_injected", map[string]any{
				"turn":    turnCount,
				"message": msg,
			})

			log.Printf("[Reminder] 已注入系统提醒")
		}

		availableTools := e.registry.GetAvailableTools()
		if e.enableThinking {
			log.Println("[Engine][Phase 1] 剥夺工具访问权，强制进入慢思考与规划阶段...")
			thinkingResponse, err := e.callModel(
				ctx,
				sess,
				recorder,
				aggregator,
				estimator,
				turnCount,
				"thinking",
				contextHistory,
				nil,
			)
			if err != nil {
				return nil, fmt.Errorf("Thinking 阶段生成失败: %w", err)
			}

			if thinkingResponse.Content != "" {
				log.Printf("🧠 [内部思考 Trace]: %s\n", thinkingResponse.Content)
				contextHistory = append(contextHistory, *thinkingResponse)
			}

		}
		log.Println("[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...")
		actionResponse, err := e.callModel(
			ctx,
			sess,
			recorder,
			aggregator,
			estimator,
			turnCount,
			"action",
			contextHistory,
			availableTools,
		)
		if err != nil {
			return nil, fmt.Errorf("模型生成失败: %w", err)
		}
		contextHistory = append(contextHistory, *actionResponse)

		if actionResponse.Content != "" {
			final = actionResponse.Content
			fmt.Printf("🤖 [对外回复]: %s\n", actionResponse.Content)
		}

		if len(actionResponse.ToolCalls) == 0 {
			log.Printf("[Engine] 模型不再需要调用工具，宣告任务完成！")
			_ = recorder.Append(aggregator.Summary(sess.ID))
			summaryWritten = true

			return &RunResult{
				FinalMessage: final,
				SessionID:    sess.ID,
			}, nil
		}

		log.Printf("[Engine] 模型请求调用 %d 个工具\n", len(actionResponse.ToolCalls))

		toolResults := e.executeToolCalls(ctx, actionResponse.ToolCalls)
		for _, item := range toolResults {
			e.reminder.Record(turnCount, item.Call, item.Result)
			e.recovery.Record(item.Call, item.Result)

			if item.Result.IsError {
				log.Printf("  -> ❌ 工具执行报错: %s, 输出：%s\n", item.Call.Name, item.Result.Output)
			} else {
				log.Printf("  -> ✅ 工具执行成功: %s（返回 %d 字节）\n", item.Call.Name, len(item.Result.Output))
			}

			_ = recorder.Append(metrics.ToolCall{
				Time:        time.Now(),
				Type:        metrics.EventToolCall,
				SessionID:   sess.ID,
				Turn:        turnCount,
				ToolName:    item.Call.Name,
				ToolCallID:  item.Call.ID,
				DurationMS:  item.DurationMS,
				OutputBytes: len(item.Result.Output),
				IsError:     item.Result.IsError,
			})
			aggregator.AddTool(item.Result.IsError)

			observationMessage := schema.Message{
				Role:       schema.RoleUser,
				Content:    item.Result.Output,
				ToolCallID: item.Call.ID,
			}
			contextHistory = append(contextHistory, observationMessage)

		}
	}
}

func (e *AgentEngine) callModel(
	ctx context.Context,
	sess *session.Session,
	recorder *metrics.Recorder,
	aggregator *metrics.Aggregator,
	estimator metrics.TokenEstimator,
	turn int,
	phase string,
	messages []schema.Message,
	tools []schema.ToolDefinition,
) (*schema.Message, error) {
	inputTokens := estimator.EstimateMessages(messages) +
		metrics.EstimateToolDefinitions(estimator, tools)

	started := time.Now()
	resp, err := e.provider.Generate(ctx, messages, tools)
	duration := time.Since(started)

	outputTokens := 0
	if resp != nil {
		outputTokens = estimator.EstimateText(resp.Content)
		for _, call := range resp.ToolCalls {
			outputTokens += estimator.EstimateText(call.Name)
			outputTokens += estimator.EstimateText(string(call.Arguments))
		}
	}

	event := metrics.ModelCall{
		Time:         time.Now(),
		Type:         metrics.EventModelCall,
		SessionID:    sess.ID,
		Turn:         turn,
		Phase:        phase,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		DurationMS:   duration.Milliseconds(),
	}

	if err != nil {
		event.Error = err.Error()
	}

	_ = recorder.Append(event)
	aggregator.AddModel(inputTokens, outputTokens, err != nil)
	return resp, err
}
