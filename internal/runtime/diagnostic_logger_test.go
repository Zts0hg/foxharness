package runtime

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
)

/* captureDiagnosticLog redirects the standard logger for one test. */
func captureDiagnosticLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buffer)
	t.Cleanup(func() { log.SetOutput(previous) })
	return &buffer
}

/* TestDiagnosticLoggerEmitsPerTurnLines verifies the baseline per-turn
 * diagnostic stream restored from the characterization baseline. */
func TestDiagnosticLoggerEmitsPerTurnLines(t *testing.T) {
	logs := captureDiagnosticLog(t)
	assembly := RunAssembly{
		Session: AgentSessionSnapshot{ID: "session-1", WorkDir: "/workspace"},
		Run:     RunScopeSnapshot{RunID: "run-1"},
	}
	logger := newDiagnosticLogger(assembly)
	baseModel := modelInvokerStub{result: engine.ModelResult{Message: schema.Message{
		Role: schema.RoleAssistant, Content: "working",
		ToolCalls: []schema.ToolCall{{ID: "call-one", Name: "inspect"}},
	}}}
	run, err := logger.wrapModel(baseModel).StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Invoke(context.Background(), engine.RunContext{Turn: 1, Phase: engine.PhaseAction}, nil); err != nil {
		t.Fatal(err)
	}
	observer := logger.wrapObserver(recordingObserver{})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactRunStarted})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactToolCall, Turn: 1, CallID: "call-one", Name: "inspect"})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactToolResult, Turn: 1, CallID: "call-one", Name: "inspect", FullContent: "tool output"})

	for _, want := range []string{
		"[Engine] 引擎启动，Session: session-1，WorkDir: /workspace",
		"====== [Turn 1] 开始",
		"[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...",
		"[Engine] 模型请求调用 1 个工具",
		"  -> ✅ 工具执行成功: inspect（返回 11 字节）",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("diagnostic log is missing %q\nlogs:\n%s", want, logs.String())
		}
	}
}

/* TestDiagnosticLoggerEmitsInjectionAndCompletionLines verifies the reminder,
 * recovery, TODO gate, and completion diagnostic lines. */
func TestDiagnosticLoggerEmitsInjectionAndCompletionLines(t *testing.T) {
	logs := captureDiagnosticLog(t)
	logger := newDiagnosticLogger(RunAssembly{
		Session: AgentSessionSnapshot{ID: "session-1"},
		Run:     RunScopeSnapshot{RunID: "run-1"},
		Spec:    RunSnapshot{Thinking: true},
	})
	observer := logger.wrapObserver(recordingObserver{})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactRunStarted})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactSystemReminder, Turn: 1, Name: string(engine.ConversationSourceReminder), Content: "reminder"})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactSystemReminder, Turn: 1, Name: string(engine.ConversationSourceTODOGate), Content: "todo"})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactErrorRecovery, Turn: 1, Content: "recovery"})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactContextCompacted, Turn: 1, Name: string(ContextCompactionPreTurn), BeforeMessages: 9, AfterMessages: 3})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactRunCompleted, Content: "done"})

	for _, want := range []string{
		"[Engine] 慢思考模式（Thinking Phase）: true",
		"[Reminder] 已注入系统提醒",
		"[TODO] Final response blocked until TODO.md is updated",
		"[Recovery] 已注入错误恢复提示",
		"[Compactor] 上下文已压缩: 9 -> 3 条消息（含 boundary + summary）",
		"[Engine] 模型不再需要调用工具，宣告任务完成！",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("diagnostic log is missing %q\nlogs:\n%s", want, logs.String())
		}
	}
}

/* TestDiagnosticLoggerScopesCompactionSuccessLineToTurnContext verifies the
 * baseline trigger scope of the compaction success line: only the pre-turn
 * (turn_context) compaction announces it, while the first-projection and
 * reactive compactions record their events without a diagnostic line. */
func TestDiagnosticLoggerScopesCompactionSuccessLineToTurnContext(t *testing.T) {
	logs := captureDiagnosticLog(t)
	observer := newDiagnosticLogger(RunAssembly{}).wrapObserver(recordingObserver{})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactContextCompacted, Turn: 1, Name: string(ContextCompactionInitialHistory), BeforeMessages: 9, AfterMessages: 3})
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactContextCompacted, Turn: 1, Name: string(ContextCompactionReactive), BeforeMessages: 3, AfterMessages: 2})
	if strings.Contains(logs.String(), "[Compactor]") {
		t.Fatalf("non-turn compaction emitted a diagnostic line:\n%s", logs.String())
	}
	observer.Observe(context.Background(), engine.Fact{Kind: engine.FactContextCompacted, Turn: 2, Name: string(ContextCompactionPreTurn), BeforeMessages: 8, AfterMessages: 2})
	if !strings.Contains(logs.String(), "[Compactor] 上下文已压缩: 8 -> 2 条消息（含 boundary + summary）") {
		t.Fatalf("turn-context compaction success line missing:\n%s", logs.String())
	}
}

/* TestDiagnosticModelEmitsPhaseLines verifies the thinking and action phase
 * announcements plus the reactive compaction notice. */
func TestDiagnosticModelEmitsPhaseLines(t *testing.T) {
	logs := captureDiagnosticLog(t)
	invocations := 0
	base := modelInvokerStub{invoke: func(_ context.Context, request engine.RunContext) (engine.ModelResult, error) {
		invocations++
		if request.Phase == engine.PhaseThinking {
			return engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "thinking trace"}}, nil
		}
		if invocations == 2 {
			return engine.ModelResult{}, engine.ErrPromptTooLong
		}
		return engine.ModelResult{Message: schema.Message{Role: schema.RoleAssistant, Content: "done"}}, nil
	}}
	run, err := newDiagnosticLogger(RunAssembly{}).wrapModel(base).StartRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.Invoke(context.Background(), engine.RunContext{Turn: 1, Phase: engine.PhaseThinking}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := run.Invoke(context.Background(), engine.RunContext{Turn: 1, Phase: engine.PhaseAction}, nil); !errors.Is(err, engine.ErrPromptTooLong) {
		t.Fatalf("Invoke() error = %v", err)
	}
	for _, want := range []string{
		"[Engine][Phase 1] 剥夺工具访问权，强制进入慢思考与规划阶段...",
		"🧠 [内部思考 Trace]: thinking trace",
		"[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...",
		"[Engine] API 拒绝请求（prompt 过长），尝试响应式压缩后重试...",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("diagnostic log is missing %q\nlogs:\n%s", want, logs.String())
		}
	}
}

/* recordingObserver swallows facts handed to the decorated observer. */
type recordingObserver struct{}

func (recordingObserver) Observe(context.Context, engine.Fact) {}

/* modelInvokerStub scripts one static result or a callback for each invocation. */
type modelInvokerStub struct {
	result engine.ModelResult
	invoke func(context.Context, engine.RunContext) (engine.ModelResult, error)
}

func (s modelInvokerStub) StartRun(context.Context) (engine.ModelRunInvoker, error) {
	return modelInvokerRunStub{stub: s}, nil
}

type modelInvokerRunStub struct{ stub modelInvokerStub }

func (r modelInvokerRunStub) Invoke(ctx context.Context, request engine.RunContext, _ engine.ModelFactEmitter) (engine.ModelResult, error) {
	if r.stub.invoke != nil {
		return r.stub.invoke(ctx, request)
	}
	return r.stub.result, nil
}

/* TestDiagnosticCompactorEmitsCompactionFailureLines verifies the baseline
 * proactive and reactive compaction failure notices: an ordinary per-turn
 * compaction failure announces the fallback to the original context, a
 * prompt-too-long retry failure announces the reactive failure, and
 * run-failing compactions stay silent because their errors propagate. */
func TestDiagnosticCompactorEmitsCompactionFailureLines(t *testing.T) {
	logs := captureDiagnosticLog(t)
	compactionErr := errors.New("summary provider failed")
	base := contextCompactorFunc(func(_ context.Context, request ContextCompactionRequest) (ContextCompactionProposal, error) {
		switch request.Trigger {
		case ContextCompactionPreTurn, ContextCompactionReactive:
			return ContextCompactionProposal{}, compactionErr
		case ContextCompactionInitialHistory:
			return ContextCompactionProposal{}, &ContextBlockedError{UsedTokens: 101, Limit: 100}
		}
		return ContextCompactionProposal{Changed: true, Messages: nil}, nil
	})
	wrapped := newDiagnosticLogger(RunAssembly{}).wrapCompactor(base)

	if _, err := wrapped.Compact(context.Background(), ContextCompactionRequest{Trigger: ContextCompactionPreTurn}); err == nil {
		t.Fatal("pre-turn Compact() error = nil, want the propagated compaction failure")
	}
	if _, err := wrapped.Compact(context.Background(), ContextCompactionRequest{Trigger: ContextCompactionReactive}); err == nil {
		t.Fatal("reactive Compact() error = nil, want the propagated compaction failure")
	}
	if _, err := wrapped.Compact(context.Background(), ContextCompactionRequest{Trigger: ContextCompactionInitialHistory}); err == nil {
		t.Fatal("initial-history Compact() error = nil, want the propagated compaction failure")
	}
	if _, err := wrapped.Compact(context.Background(), ContextCompactionRequest{Trigger: ContextCompactionManual}); err != nil {
		t.Fatalf("manual Compact() error = %v", err)
	}
	for _, want := range []string{
		"[Compactor] 压缩失败，将继续使用原始上下文: summary provider failed",
		"[Compactor] 响应式压缩失败: summary provider failed",
	} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("diagnostic log is missing %q\nlogs:\n%s", want, logs.String())
		}
	}
	if strings.Count(logs.String(), "[Compactor]") != 2 {
		t.Fatalf("compaction failure log emitted unexpected extra lines:\n%s", logs.String())
	}
}
