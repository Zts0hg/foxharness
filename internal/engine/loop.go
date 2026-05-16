// Package engine provides the core agent execution loop for foxharness.
//
// The engine orchestrates tool-using LLM agents through a turn-based reasoning
// process. Each turn consists of an optional Thinking phase (for planning)
// followed by an Action phase where tools may be invoked. The Loop manages
// context history, tool execution, result aggregation, and integrates with
// compaction, error recovery, and system reminders.
//
// Key Components:
//   - AgentEngine: Main execution engine managing turns and context
//   - Config: Engine configuration for thinking mode and turn limits
//   - PromptComposer: Interface for composing system prompts
//
// The engine supports parallel-safe tool execution, context compaction when
// approaching token limits, automatic error recovery prompt injection, and
// comprehensive tracing and metrics collection.
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
	"github.com/Zts0hg/foxharness/internal/tracing"
)

// PromptComposer defines the interface for composing system prompts
// from user input. Implementations can inject system instructions,
// context, and formatting into the prompt sent to the LLM.
type PromptComposer interface {
	// Compose creates a system prompt from the user's input.
	// The returned prompt should include any necessary system instructions,
	// context, or formatting required for the LLM to process the user's request.
	Compose(userPrompt string) (string, error)
}

// RunResult contains the final output from an engine run.
// It provides the agent's final message and the associated session ID.
type RunResult struct {
	// FinalMessage is the last text content produced by the agent.
	FinalMessage string
	// SessionID uniquely identifies the session that produced this result.
	SessionID string
}

// AgentEngine manages the main agent execution loop with turn-based reasoning.
// It orchestrates the flow between the LLM provider, tool registry, and
// supporting systems (compaction, recovery, reminders, tracing, metrics).
//
// The engine executes turns until:
//   - The agent produces a response with no tool calls
//   - The maximum turn limit is reached
//   - A fatal error occurs
//
// Each turn can optionally include a Thinking phase (for planning) followed
// by an Action phase where tools may be invoked. Tool calls are executed
// with parallelization support for tools marked as parallel-safe.
type AgentEngine struct {
	// provider is the LLM provider for generating responses.
	provider provider.LLMProvider
	// registry manages available tools and executes tool calls.
	registry tools.Registry
	// workDir is the working directory for file-based operations.
	workDir string
	// composer creates system prompts from user input.
	composer PromptComposer
	// config contains engine behavior configuration.
	config Config
	// compactor optionally compresses context history when approaching limits.
	compactor *compaction.Compactor
	// recovery tracks tool failures and injects recovery prompts.
	recovery *recovery.Tracker
	// reminder manages system reminder injection based on tool patterns.
	reminder *reminder.Manager
}

type indexedToolResult struct {
	Index      int
	Call       schema.ToolCall
	Result     schema.ToolResult
	DurationMS int64
}

// NewAgentEngine creates a new AgentEngine with the provided configuration.
//
// The p parameter is the LLM provider for generating responses.
// The r parameter is the tool registry for managing and executing tools.
// The workDir parameter specifies the working directory for file operations.
// The composer parameter creates system prompts from user input.
// The config parameter controls engine behavior; if MaxTurns is <= 0, it defaults to 20.
//
// Returns a configured AgentEngine ready to run agent sessions.
func NewAgentEngine(
	p provider.LLMProvider,
	r tools.Registry,
	workDir string,
	composer PromptComposer,
	config Config,
) *AgentEngine {
	if config.MaxTurns <= 0 {
		config.MaxTurns = 20
	}

	return &AgentEngine{
		provider: p,
		registry: r,
		workDir:  workDir,
		composer: composer,
		config:   config,
		recovery: recovery.NewTracker(),
		reminder: reminder.NewManager(),
	}
}

// WithCompactor sets the context compactor for the engine.
// The compactor is invoked at the start of each turn to potentially
// compress the conversation history when approaching token limits.
// If c is nil, no compaction is performed.
func (e *AgentEngine) WithCompactor(c *compaction.Compactor) {
	e.compactor = c
}

func (e *AgentEngine) executeToolCalls(ctx context.Context, tracer *tracing.Tracer, parentSpanID string, calls []schema.ToolCall) []indexedToolResult {
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
				span := tracer.StartSpan(parentSpanID, "tool_call", map[string]any{
					"tool":         call.Name,
					"tool_call_id": call.ID,
				})
				started := time.Now()
				result := e.registry.Execute(ctx, call)
				durationMS := time.Since(started).Milliseconds()
				results[idx] = indexedToolResult{
					Index:      idx,
					Call:       call,
					Result:     result,
					DurationMS: durationMS,
				}

				status := "ok"
				if result.IsError {
					status = "error"
				}
				span.End(status, map[string]any{
					"is_error":     result.IsError,
					"output_bytes": len(result.Output),
				})
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

		span := tracer.StartSpan(parentSpanID, "tool_call", map[string]any{
			"tool":         call.Name,
			"tool_call_id": call.ID,
		})
		started := time.Now()
		result := e.registry.Execute(ctx, call)
		durationMS := time.Since(started).Milliseconds()
		results[i] = indexedToolResult{
			Index:      i,
			Call:       call,
			Result:     result,
			DurationMS: durationMS,
		}

		status := "ok"
		if result.IsError {
			status = "error"
		}
		span.End(status, map[string]any{
			"is_error":     result.IsError,
			"output_bytes": len(result.Output),
		})
	}

	flushParallelBatch(parallelBatch)
	return results
}

// Run executes the agent loop with the provided session and user prompt.
//
// The ctx parameter provides cancellation support for the entire run.
// The sess parameter contains the session state for persistence and tracking.
// The userPrompt parameter is the initial user request to process.
//
// The engine executes turns, each consisting of:
//  1. Optional context compaction (if compactor is configured)
//  2. Optional error recovery prompt injection
//  3. Optional system reminder injection
//  4. Optional Thinking phase (if EnableThinking is true)
//  5. Action phase with tool access
//
// The loop continues until:
//   - The agent produces a response with no tool calls
//   - The maximum turn limit (Config.MaxTurns) is reached
//   - A fatal error occurs
//
// Returns a RunResult containing the final agent message and session ID,
// or an error if the run fails catastrophically.
func (e *AgentEngine) Run(ctx context.Context, sess *session.Session, userPrompt string) (*RunResult, error) {
	return e.RunWithReporter(ctx, sess, userPrompt, nil)
}

// RunWithReporter executes the agent loop and streams lifecycle events to
// reporter when one is provided. Passing nil keeps the legacy CLI-oriented
// behavior.
func (e *AgentEngine) RunWithReporter(ctx context.Context, sess *session.Session, userPrompt string, reporter Reporter) (*RunResult, error) {
	log.Printf("[Engine] 引擎启动，Session: %s，WorkDir: %s\n", sess.ID, e.workDir)
	log.Printf("[Engine] 慢思考模式（Thinking Phase）: %v\n", e.config.EnableThinking)

	tracer := tracing.NewTracer(sess.TracePath())
	runSpan := tracer.StartSpan("", "run", map[string]any{
		"session_id": sess.ID,
		"source":     sess.Source,
		"work_dir":   sess.WorkDir,
	})
	runStatus := "ok"
	runAttrs := map[string]any{}
	defer func() {
		runSpan.End(runStatus, runAttrs)
	}()

	markRunError := func(err error) {
		runStatus = "error"
		runAttrs["error"] = err.Error()
	}

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
		wrapped := fmt.Errorf("组装系统提示词失败: %w", err)
		markRunError(wrapped)
		return nil, wrapped
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
		if e.config.MaxTurns > 0 && turnCount > e.config.MaxTurns {
			wrapped := fmt.Errorf("超过最大 Turn 数限制: %d", e.config.MaxTurns)
			markRunError(wrapped)
			return &RunResult{
				FinalMessage: final,
				SessionID:    sess.ID,
			}, wrapped
		}
		log.Printf("====== [Turn %d] 开始", turnCount)
		turnSpan := tracer.StartSpan(runSpan.ID(), "turn", map[string]any{
			"turn": turnCount,
		})
		turnEnded := false
		finishTurn := func(status string, attrs map[string]any) {
			if turnEnded {
				return
			}
			turnEnded = true
			turnSpan.End(status, attrs)
		}

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

			// tracer.Annotate(turnSpan.ID(), "context_compacted", map[string]any{
			// 	"before_messages": before,
			// 	"after_messages":  after,
			// })
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

				tracer.Annotate(turnSpan.ID(), "error_recovery_injected", map[string]any{
					"turn": turnCount,
				})
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

			tracer.Annotate(turnSpan.ID(), "system_reminder_injected", map[string]any{
				"turn": turnCount,
			})
		}

		availableTools := e.registry.GetAvailableTools()
		if e.config.EnableThinking {
			log.Println("[Engine][Phase 1] 剥夺工具访问权，强制进入慢思考与规划阶段...")
			if reporter != nil {
				reporter.OnThinking(ctx, turnCount)
			}
			thinkingResponse, err := e.callModel(
				ctx,
				sess,
				recorder,
				aggregator,
				estimator,
				tracer,
				turnSpan.ID(),
				turnCount,
				"thinking",
				contextHistory,
				nil,
			)
			if err != nil {
				wrapped := fmt.Errorf("Thinking 阶段生成失败: %w", err)
				finishTurn("error", map[string]any{"error": wrapped.Error()})
				markRunError(wrapped)
				return nil, wrapped
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
			tracer,
			turnSpan.ID(),
			turnCount,
			"action",
			contextHistory,
			availableTools,
		)
		if err != nil {
			wrapped := fmt.Errorf("模型生成失败: %w", err)
			finishTurn("error", map[string]any{"error": wrapped.Error()})
			markRunError(wrapped)
			return nil, wrapped
		}
		contextHistory = append(contextHistory, *actionResponse)

		if actionResponse.Content != "" {
			final = actionResponse.Content
			if reporter != nil {
				reporter.OnMessage(ctx, actionResponse.Content)
			} else {
				fmt.Printf("🤖 [对外回复]: %s\n", actionResponse.Content)
			}
		}

		if len(actionResponse.ToolCalls) == 0 {
			log.Printf("[Engine] 模型不再需要调用工具，宣告任务完成！")
			_ = recorder.Append(aggregator.Summary(sess.ID))
			summaryWritten = true

			finishTurn("ok", map[string]any{
				"tool_calls": 0,
				"final":      true,
			})

			return &RunResult{
				FinalMessage: final,
				SessionID:    sess.ID,
			}, nil
		}

		log.Printf("[Engine] 模型请求调用 %d 个工具\n", len(actionResponse.ToolCalls))
		if reporter != nil {
			for _, toolCall := range actionResponse.ToolCalls {
				reporter.OnToolCall(ctx, toolCall.Name, string(toolCall.Arguments))
			}
		}

		toolResults := e.executeToolCalls(ctx, tracer, turnSpan.ID(), actionResponse.ToolCalls)
		for _, item := range toolResults {
			e.reminder.Record(turnCount, item.Call, item.Result)
			e.recovery.Record(item.Call, item.Result)
			if reporter != nil {
				reporter.OnToolResult(
					ctx,
					item.Call.Name,
					truncateReporterOutput(item.Result.Output, 800),
					item.Result.IsError,
				)
			}

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

		finishTurn("ok", map[string]any{
			"tool_calls": len(actionResponse.ToolCalls),
		})
	}
}

func truncateReporterOutput(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return fmt.Sprintf("%s\n... (已截断，原始输出约 %d 字节)", string(runes[:limit]), len(s))
}

func (e *AgentEngine) callModel(
	ctx context.Context,
	sess *session.Session,
	recorder *metrics.Recorder,
	aggregator *metrics.Aggregator,
	estimator metrics.TokenEstimator,
	tracer *tracing.Tracer,
	parentSpanID string,
	turn int,
	phase string,
	messages []schema.Message,
	tools []schema.ToolDefinition,
) (*schema.Message, error) {
	span := tracer.StartSpan(parentSpanID, "model_call", map[string]any{
		"phase":       phase,
		"turn":        turn,
		"message_len": len(messages),
		"tools":       len(tools),
	})

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
	if err != nil {
		span.End("error", map[string]any{"error": err.Error()})
		return nil, err
	}
	span.End("ok", map[string]any{
		"content_bytes": len(resp.Content),
		"tool_calls":    len(resp.ToolCalls),
	})

	return resp, nil
}
