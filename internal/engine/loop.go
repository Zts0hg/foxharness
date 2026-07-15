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
	"strconv"
	"strings"
	"time"

	"sync"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/metrics"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/recovery"
	"github.com/Zts0hg/foxharness/internal/reminder"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolresult"
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
	// RunID uniquely identifies this user-submitted run within the session.
	RunID string
	// MetricsPath is the run-local metrics file.
	MetricsPath string
	// TracePath is the run-local trace file.
	TracePath string
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
	// fs is the FileSystem used for tool-result persistence. Defaults to
	// toolresult.OSFileSystem; tests can swap this via WithFileSystem.
	fs toolresult.FileSystem
}

type providerMetadata interface {
	ProviderProtocol() string
	ModelName() string
}

type indexedToolResult struct {
	Index      int
	Call       schema.ToolCall
	Result     schema.ToolResult
	DurationMS int64
}

// processedToolResult bundles a raw tool execution with the content that
// should be appended to the conversation history. ContextContent is either
// the (post-truncation) full output for small results, or a preview that
// references the persisted on-disk copy for large outputs.
type processedToolResult struct {
	indexedToolResult
	ContextContent string
	Persisted      bool
}

// NewAgentEngine creates a new AgentEngine with the provided configuration.
//
// The p parameter is the LLM provider for generating responses.
// The r parameter is the tool registry for managing and executing tools.
// The workDir parameter specifies the working directory for file operations.
// The composer parameter creates system prompts from user input.
// The config parameter controls engine behavior; if MaxTurns is <= 0, there is no turn limit.
//
// Returns a configured AgentEngine ready to run agent sessions.
func NewAgentEngine(
	p provider.LLMProvider,
	r tools.Registry,
	workDir string,
	composer PromptComposer,
	config Config,
) *AgentEngine {
	if metadata, ok := p.(providerMetadata); ok {
		if config.ProviderProtocol == "" {
			config.ProviderProtocol = metadata.ProviderProtocol()
		}
		if config.Model == "" {
			config.Model = metadata.ModelName()
		}
	}

	return &AgentEngine{
		provider: p,
		registry: r,
		workDir:  workDir,
		composer: composer,
		config:   config,
		recovery: recovery.NewTracker(),
		reminder: reminder.NewManager(),
		fs:       toolresult.OSFileSystem{},
	}
}

// WithCompactor sets the context compactor for the engine.
// The compactor is invoked at the start of each turn to potentially
// compress the conversation history when approaching token limits.
// If c is nil, no compaction is performed.
func (e *AgentEngine) WithCompactor(c *compaction.Compactor) {
	e.compactor = c
}

// WithFileSystem swaps the FileSystem used for tool-result persistence.
// The default (toolresult.OSFileSystem) writes to the session directory;
// tests can inject an in-memory implementation to avoid disk I/O. Passing
// nil leaves the existing FileSystem unchanged.
func (e *AgentEngine) WithFileSystem(fs toolresult.FileSystem) {
	if fs != nil {
		e.fs = fs
	}
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
				if e.config.OnToolCalled != nil {
					e.config.OnToolCalled(call, result)
				}
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
		if e.config.OnToolCalled != nil {
			e.config.OnToolCalled(call, result)
		}
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
// reporter when one is provided. The engine does not write user-facing output
// directly to the terminal; callers are responsible for presenting the
// returned final message or reporter events.
func (e *AgentEngine) RunWithReporter(ctx context.Context, sess *session.Session, userPrompt string, reporter Reporter) (*RunResult, error) {
	log.Printf("[Engine] 引擎启动，Session: %s，WorkDir: %s\n", sess.ID, e.workDir)
	log.Printf("[Engine] 慢思考模式（Thinking Phase）: %v\n", e.config.EnableThinking)

	run, err := sess.StartRun(userPrompt)
	if err != nil {
		return nil, err
	}
	var runErr error
	var finalResult *RunResult
	if reporter != nil {
		reporter.OnRunStart(ctx, sess.ID, run.ID)
	}
	defer func() {
		if err := run.Finish(); err != nil {
			log.Printf("[Engine] 写入 Run 完成状态失败: %v", err)
		}
	}()
	defer func() {
		if reporter == nil {
			return
		}
		if runErr != nil {
			reporter.OnRunError(ctx, sess.ID, run.ID, runErr)
			return
		}
		if finalResult != nil {
			reporter.OnRunComplete(ctx, *finalResult)
		}
	}()

	tracer := tracing.NewTracer(run.TracePath())
	runSpan := tracer.StartSpan("", "run", map[string]any{
		"session_id": sess.ID,
		"run_id":     run.ID,
		"source":     sess.Source,
		"work_dir":   sess.WorkDir,
	})
	runStatus := "ok"
	runAttrs := map[string]any{}
	defer func() {
		runSpan.End(runStatus, runAttrs)
	}()

	markRunError := func(err error) {
		runErr = err
		runStatus = "error"
		runAttrs["error"] = err.Error()
	}

	recorder := metrics.NewRecorder(run.MetricsPath())
	aggregator := metrics.NewAggregator()
	estimator := metrics.RoughEstimator{}

	summaryWritten := false
	defer func() {
		if !summaryWritten {
			_ = recorder.Append(aggregator.Summary(sess.ID))
		}
	}()

	transcript := session.NewTranscript(sess)
	messageLog := session.NewMessageLog(sess)
	history, err := messageLog.LoadRecords()
	if err != nil {
		wrapped := fmt.Errorf("读取 Session 消息历史失败: %w", err)
		markRunError(wrapped)
		return nil, wrapped
	}
	displayPrompt := strings.TrimSpace(e.config.DisplayPrompt)
	if displayPrompt == "" {
		displayPrompt = userPrompt
	}
	promptPayload := map[string]string{"prompt": displayPrompt}
	if displayPrompt != userPrompt {
		promptPayload["model_prompt"] = userPrompt
	}
	_ = transcript.AppendRun(run.ID, "user_prompt", promptPayload)

	systemPrompt, err := e.composer.Compose(userPrompt)
	if err != nil {
		wrapped := fmt.Errorf("组装系统提示词失败: %w", err)
		markRunError(wrapped)
		return nil, wrapped
	}

	userMessage := schema.Message{
		Role:    schema.RoleUser,
		Content: userPrompt,
	}
	userSeq, err := messageLog.AppendWithDisplay(run.ID, userMessage, e.config.DisplayPrompt)
	if err != nil {
		wrapped := fmt.Errorf("写入 Session 用户消息失败: %w", err)
		markRunError(wrapped)
		return nil, wrapped
	}
	userMessageID := strconv.FormatInt(userSeq, 10)
	if e.config.OnUserMessageID != nil {
		e.config.OnUserMessageID(userMessageID)
	}
	if e.config.Checkpointer != nil {
		if err := e.config.Checkpointer.MakeSnapshot(userMessageID); err != nil {
			log.Printf("[Checkpoint] 创建快照失败，将继续执行: %v", err)
		}
	}

	contextHistory, compactedInitial, err := e.buildInitialContext(ctx, sess, systemPrompt, history, userMessage)
	if err != nil {
		wrapped := fmt.Errorf("组装 Session 上下文失败: %w", err)
		markRunError(wrapped)
		return nil, wrapped
	}
	if compactedInitial {
		_ = transcript.AppendRun(run.ID, "context_compacted", map[string]any{
			"scope": "session_history",
		})
		if reporter != nil {
			reporter.OnCompaction(ctx, "session_history")
		}
	}

	turnCount := 0
	final := ""
	todoUpdated := false
	todoGateReminderSent := false
	completionGateReminderSent := ""

	for {
		turnCount++
		if e.config.MaxTurns > 0 && turnCount > e.config.MaxTurns {
			wrapped := fmt.Errorf("超过最大 Turn 数限制: %d", e.config.MaxTurns)
			markRunError(wrapped)
			return &RunResult{
				FinalMessage: final,
				SessionID:    sess.ID,
				RunID:        run.ID,
				MetricsPath:  run.MetricsPath(),
				TracePath:    run.TracePath(),
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

		if turnAware, ok := e.registry.(tools.TurnAwareRegistry); ok {
			turnAware.BeginTurn()
		}
		availableTools := e.registry.GetAvailableTools()
		toolTokens := estimateToolTokens(estimator, availableTools)

		justCompacted := false
		if e.compactor != nil {
			e.compactor.SetToolOverhead(toolTokens)
			compacted, err := e.compactor.MaybeCompact(ctx, contextHistory)
			if err != nil {
				log.Printf("[Compactor] 压缩失败，将继续使用原始上下文: %v", err)
			} else if !sameMessages(compacted, contextHistory) {
				log.Printf("[Compactor] 上下文已压缩: %d -> %d 条消息（含 boundary + summary）", len(contextHistory), len(compacted))
				contextHistory = compacted
				justCompacted = true
				_ = transcript.AppendRun(run.ID, "context_compacted", map[string]any{
					"turn": turnCount,
				})
				if reporter != nil {
					reporter.OnCompaction(ctx, "turn_context")
				}
			}
			if e.config.OnContextEstimate != nil {
				used := e.compactor.Estimate(contextHistory) + toolTokens
				e.config.OnContextEstimate(used, e.compactor.ContextWindow())
			}
		}
		if e.recovery.ShouldInject() {
			recoveryPrompt := e.recovery.BuildPrompt()
			if recoveryPrompt != "" {
				contextHistory = append(contextHistory, schema.Message{
					Role:    schema.RoleUser,
					Content: "[Runtime System Notice]\n\n" + recoveryPrompt,
				})

				_ = transcript.AppendRun(run.ID, "error_recovery_injected", map[string]any{
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

			_ = transcript.AppendRun(run.ID, "system_reminder_injected", map[string]any{
				"turn":    turnCount,
				"message": msg,
			})

			log.Printf("[Reminder] 已注入系统提醒")

			tracer.Annotate(turnSpan.ID(), "system_reminder_injected", map[string]any{
				"turn": turnCount,
			})
		}

		if e.config.NextTurnReminders != nil {
			for _, extra := range e.config.NextTurnReminders() {
				if extra == "" {
					continue
				}
				contextHistory = append(contextHistory, schema.Message{
					Role:    schema.RoleUser,
					Content: "[Runtime System Reminder]\n\n" + extra,
				})
				_ = transcript.AppendRun(run.ID, "system_reminder_injected", map[string]any{
					"turn":    turnCount,
					"message": extra,
					"source":  "next_turn_reminders",
				})
				tracer.Annotate(turnSpan.ID(), "system_reminder_injected", map[string]any{
					"turn":   turnCount,
					"source": "next_turn_reminders",
				})
			}
		}

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
		if e.compactor != nil && !justCompacted {
			used := e.compactor.Estimate(contextHistory)
			blocking := e.compactor.BlockingThreshold()
			if used >= blocking {
				wrapped := fmt.Errorf("上下文 token 数 (%d) 超过阻塞阈值 (%d)，无法继续发送请求", used, blocking)
				finishTurn("error", map[string]any{"error": wrapped.Error()})
				markRunError(wrapped)
				return &RunResult{
					FinalMessage: final,
					SessionID:    sess.ID,
					RunID:        run.ID,
					MetricsPath:  run.MetricsPath(),
					TracePath:    run.TracePath(),
				}, wrapped
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
		if err != nil && e.compactor != nil && provider.IsPromptTooLong(err) {
			log.Printf("[Engine] API 拒绝请求（prompt 过长），尝试响应式压缩后重试...")
			reactiveCompacted, compactErr := e.compactor.ForceCompact(ctx, contextHistory)
			if compactErr != nil {
				log.Printf("[Compactor] 响应式压缩失败: %v", compactErr)
			} else if !sameMessages(reactiveCompacted, contextHistory) {
				contextHistory = reactiveCompacted
				_ = transcript.AppendRun(run.ID, "context_compacted", map[string]any{
					"turn":   turnCount,
					"source": "reactive",
				})
				if reporter != nil {
					reporter.OnCompaction(ctx, "reactive")
				}
				actionResponse, err = e.callModel(
					ctx, sess, recorder, aggregator, estimator, tracer,
					turnSpan.ID(), turnCount, "action",
					contextHistory, availableTools,
				)
			}
		}
		if err != nil {
			wrapped := fmt.Errorf("模型生成失败: %w", err)
			finishTurn("error", map[string]any{"error": wrapped.Error()})
			markRunError(wrapped)
			return nil, wrapped
		}
		if e.compactor != nil && actionResponse.Usage != nil {
			e.compactor.ResetCircuitBreaker()
		}
		contextHistory = append(contextHistory, *actionResponse)
		if _, err := messageLog.Append(run.ID, *actionResponse); err != nil {
			wrapped := fmt.Errorf("写入 Session 助手消息失败: %w", err)
			finishTurn("error", map[string]any{"error": wrapped.Error()})
			markRunError(wrapped)
			return nil, wrapped
		}

		if actionResponse.Content != "" {
			final = actionResponse.Content
			if reporter != nil {
				reporter.OnMessage(ctx, actionResponse.Content)
			}
		}

		if len(actionResponse.ToolCalls) == 0 {
			if e.config.CompletionGate != nil {
				if reminder := strings.TrimSpace(e.config.CompletionGate()); reminder != "" {
					if completionGateReminderSent == reminder {
						wrapped := fmt.Errorf("completion gate remained unsatisfied after reminder: %s", reminder)
						finishTurn("error", map[string]any{"error": wrapped.Error()})
						markRunError(wrapped)
						return &RunResult{
							FinalMessage: final,
							SessionID:    sess.ID,
							RunID:        run.ID,
							MetricsPath:  run.MetricsPath(),
							TracePath:    run.TracePath(),
						}, wrapped
					}
					completionGateReminderSent = reminder
					contextHistory = append(contextHistory, schema.Message{
						Role:    schema.RoleUser,
						Content: "[Runtime System Reminder]\n\n" + reminder,
					})
					_ = transcript.AppendRun(run.ID, "system_reminder_injected", map[string]any{
						"turn":    turnCount,
						"message": reminder,
						"source":  "completion_gate",
					})
					finishTurn("ok", map[string]any{
						"tool_calls": 0,
						"final":      false,
						"blocked_by": "completion_gate",
					})
					continue
				}
			}
			if reminder := e.todoCompletionReminder(sess); reminder != "" && !todoUpdated {
				if todoGateReminderSent {
					wrapped := fmt.Errorf("TODO.md still has incomplete checklist items after TODO completion reminder")
					finishTurn("error", map[string]any{"error": wrapped.Error()})
					markRunError(wrapped)
					return nil, wrapped
				}
				todoGateReminderSent = true
				contextHistory = append(contextHistory, schema.Message{
					Role:    schema.RoleUser,
					Content: todoCompletionReminderMessage(reminder),
				})
				_ = transcript.AppendRun(run.ID, "system_reminder_injected", map[string]any{
					"turn":    turnCount,
					"message": reminder,
					"source":  "todo_completion_gate",
				})
				log.Printf("[TODO] Final response blocked until TODO.md is updated")
				finishTurn("ok", map[string]any{
					"tool_calls": 0,
					"final":      false,
					"blocked_by": "todo_completion_gate",
				})
				continue
			}
			log.Printf("[Engine] 模型不再需要调用工具，宣告任务完成！")
			_ = recorder.Append(aggregator.Summary(sess.ID))
			summaryWritten = true

			finishTurn("ok", map[string]any{
				"tool_calls": 0,
				"final":      true,
			})

			result := &RunResult{
				FinalMessage: final,
				SessionID:    sess.ID,
				RunID:        run.ID,
				MetricsPath:  run.MetricsPath(),
				TracePath:    run.TracePath(),
			}
			finalResult = result
			return result, nil
		}

		log.Printf("[Engine] 模型请求调用 %d 个工具\n", len(actionResponse.ToolCalls))
		if reporter != nil {
			detailed, _ := reporter.(DetailedReporter)
			for _, toolCall := range actionResponse.ToolCalls {
				reporter.OnToolCall(ctx, toolCall.Name, string(toolCall.Arguments))
				if detailed != nil {
					detailed.OnToolCallDetail(ctx, toolCall)
				}
			}
		}

		processed := e.processToolResults(sess, contextHistory, e.executeToolCalls(ctx, tracer, turnSpan.ID(), actionResponse.ToolCalls))

		for _, item := range processed {
			e.reminder.Record(turnCount, item.Call, item.Result)
			e.recovery.Record(item.Call, item.Result)
			if reporter != nil {
				reporter.OnToolResult(
					ctx,
					item.Call.Name,
					truncateReporterOutput(item.Result.Output, 800),
					item.Result.IsError,
				)
				if detailed, ok := reporter.(DetailedReporter); ok {
					detailed.OnToolResultDetail(ctx, item.Call, item.Result)
				}
			}

			if item.Result.IsError {
				log.Printf("  -> ❌ 工具执行报错: %s, 输出：%s\n", item.Call.Name, item.Result.Output)
			} else {
				log.Printf("  -> ✅ 工具执行成功: %s（返回 %d 字节）\n", item.Call.Name, len(item.Result.Output))
				if item.Call.Name == "update_todo" {
					todoUpdated = true
				}
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
				Content:    item.ContextContent,
				ToolCallID: item.Call.ID,
			}
			contextHistory = append(contextHistory, observationMessage)
			if _, err := messageLog.Append(run.ID, observationMessage); err != nil {
				wrapped := fmt.Errorf("写入 Session 工具结果失败: %w", err)
				finishTurn("error", map[string]any{"error": wrapped.Error()})
				markRunError(wrapped)
				return nil, wrapped
			}

		}

		finishTurn("ok", map[string]any{
			"tool_calls": len(actionResponse.ToolCalls),
		})
	}
}

// processToolResults applies the absolute size cap, persists oversize results
// to disk, and enforces the per-turn budget before returning the per-result
// content that should be inserted into the conversation history.
//
// seenIDs is derived from contextHistory at call time and lists tool result
// IDs the model has already observed in earlier turns; these are excluded
// from budget-driven retroactive persistence so the prompt cache stays
// consistent. The current turn's new results are not yet in contextHistory,
// so they remain eligible for persistence. This relies on tool call IDs
// being unique per call — providers guarantee this within a single response.
func (e *AgentEngine) processToolResults(
	sess *session.Session,
	contextHistory []schema.Message,
	raw []indexedToolResult,
) []processedToolResult {
	for i := range raw {
		raw[i].Result.Output = toolresult.TruncateToCap(raw[i].Result.Output)
	}

	seenIDs := make(map[string]bool, len(contextHistory))
	for _, msg := range contextHistory {
		if msg.ToolCallID != "" {
			seenIDs[msg.ToolCallID] = true
		}
	}

	dir := sess.ToolResultsDir()
	persisted := make([]toolresult.PersistedResult, len(raw))
	for i, item := range raw {
		persisted[i] = toolresult.PersistIfNeeded(e.fs, dir, item.Result)
	}
	persisted = toolresult.EnforceBudget(e.fs, dir, persisted, seenIDs)

	out := make([]processedToolResult, len(raw))
	for i, item := range raw {
		out[i] = processedToolResult{
			indexedToolResult: item,
			ContextContent:    persisted[i].Preview,
			Persisted:         persisted[i].Persisted,
		}
	}
	return out
}

// sameMessages reports whether two message slices refer to the same backing
// array — the cheap "did compaction return the input unchanged?" check. We
// cannot rely on len-equality because a compaction round can leave the count
// unchanged (e.g., recent-keep window matches the input size) while still
// mutating content; conversely the new format can produce more messages than
// the input thanks to the boundary marker.
func sameMessages(a, b []schema.Message) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	return &a[0] == &b[0]
}

func estimateToolTokens(est metrics.TokenEstimator, tools []schema.ToolDefinition) int {
	return metrics.EstimateToolDefinitions(est, tools)
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
	attrs := map[string]any{
		"phase":       phase,
		"turn":        turn,
		"message_len": len(messages),
		"tools":       len(tools),
	}
	if e.config.ProviderProtocol != "" {
		attrs["provider_protocol"] = e.config.ProviderProtocol
	}
	if e.config.Model != "" {
		attrs["model"] = e.config.Model
	}
	span := tracer.StartSpan(parentSpanID, "model_call", attrs)

	inputTokens := estimator.EstimateMessages(messages) +
		metrics.EstimateToolDefinitions(estimator, tools)

	started := time.Now()
	resp, err := e.generate(ctx, messages, tools)
	duration := time.Since(started)
	var message *schema.Message
	if resp != nil && resp.Message != nil {
		normalized := schema.NormalizeMessage(*resp.Message)
		message = &normalized
		usage := resp.Usage
		message.Usage = &usage
	}

	outputTokens := 0
	if message != nil {
		outputTokens = estimator.EstimateText(message.Content)
		for _, call := range message.ToolCalls {
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
	if message == nil {
		err := fmt.Errorf("provider returned empty response")
		span.End("error", map[string]any{"error": err.Error()})
		return nil, err
	}
	span.End("ok", map[string]any{
		"content_bytes": len(message.Content),
		"tool_calls":    len(message.ToolCalls),
	})

	return message, nil
}

func (e *AgentEngine) generate(ctx context.Context, messages []schema.Message, tools []schema.ToolDefinition) (*provider.GenerateResponse, error) {
	effort := strings.TrimSpace(e.config.EffortOverride)
	if effort == "" {
		return e.provider.Generate(ctx, messages, tools)
	}
	withOptions, ok := e.provider.(provider.OptionsGenerator)
	if !ok {
		return nil, fmt.Errorf("provider does not support effort options")
	}
	return withOptions.GenerateWithOptions(ctx, messages, tools, provider.GenerateOptions{Effort: effort})
}
