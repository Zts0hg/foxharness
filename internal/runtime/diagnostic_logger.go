package runtime

import (
	"context"
	"errors"
	"log"

	"github.com/Zts0hg/foxharness/internal/engine"
)

/*
diagnosticLogger restores the baseline per-turn stderr diagnostic stream for
one run. It derives the historical engine announcements — turn boundaries,
model phases, tool rounds, context compaction, injected reminders and recovery
notices, and completion — from the canonical run observation points instead of
logging inside the target engine.
*/
type diagnosticLogger struct {
	assembly RunAssembly
	turn     int
}

/* newDiagnosticLogger creates the run-scoped diagnostic decorator. */
func newDiagnosticLogger(assembly RunAssembly) *diagnosticLogger {
	return &diagnosticLogger{assembly: assembly}
}

/* wrapModel decorates the model invoker with the phase announcement lines. */
func (d *diagnosticLogger) wrapModel(base engine.ModelInvoker) engine.ModelInvoker {
	return diagnosticModel{logger: d, base: base}
}

/* wrapObserver decorates the run observer with the fact-derived lines. */
func (d *diagnosticLogger) wrapObserver(base engine.Observer) engine.Observer {
	return &diagnosticObserver{logger: d, base: base}
}

type diagnosticModel struct {
	logger *diagnosticLogger
	base   engine.ModelInvoker
}

func (m diagnosticModel) StartRun(ctx context.Context) (engine.ModelRunInvoker, error) {
	run, err := m.base.StartRun(ctx)
	if err != nil {
		return nil, err
	}
	return diagnosticModelRun{logger: m.logger, base: run}, nil
}

type diagnosticModelRun struct {
	logger *diagnosticLogger
	base   engine.ModelRunInvoker
}

func (r diagnosticModelRun) Invoke(ctx context.Context, request engine.RunContext, emit engine.ModelFactEmitter) (engine.ModelResult, error) {
	if request.Turn > r.logger.turn {
		r.logger.turn = request.Turn
		log.Printf("====== [Turn %d] 开始", request.Turn)
	}
	switch request.Phase {
	case engine.PhaseThinking:
		log.Println("[Engine][Phase 1] 剥夺工具访问权，强制进入慢思考与规划阶段...")
	case engine.PhaseAction:
		log.Println("[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...")
	}
	result, err := r.base.Invoke(ctx, request, emit)
	if errors.Is(err, engine.ErrPromptTooLong) {
		log.Printf("[Engine] API 拒绝请求（prompt 过长），尝试响应式压缩后重试...")
	}
	if err != nil {
		return result, err
	}
	if request.Phase == engine.PhaseThinking && result.Message.Content != "" {
		log.Printf("🧠 [内部思考 Trace]: %s\n", result.Message.Content)
	}
	if request.Phase == engine.PhaseAction && len(result.Message.ToolCalls) > 0 {
		log.Printf("[Engine] 模型请求调用 %d 个工具\n", len(result.Message.ToolCalls))
	}
	return result, nil
}

type diagnosticObserver struct {
	logger *diagnosticLogger
	base   engine.Observer
}

func (o *diagnosticObserver) Observe(ctx context.Context, fact engine.Fact) {
	switch fact.Kind {
	case engine.FactRunStarted:
		log.Printf("[Engine] 引擎启动，Session: %s，WorkDir: %s\n", o.logger.assembly.Session.ID, o.logger.assembly.Session.WorkDir)
		log.Printf("[Engine] 慢思考模式（Thinking Phase）: %v\n", o.logger.assembly.Spec.Thinking)
	case engine.FactContextCompacted:
		log.Printf("[Compactor] 上下文已压缩（trigger: %s）", fact.Name)
	case engine.FactToolResult:
		if fact.IsError {
			log.Printf("  -> ❌ 工具执行报错: %s, 输出：%s\n", fact.Name, fact.FullContent)
		} else {
			log.Printf("  -> ✅ 工具执行成功: %s（返回 %d 字节）\n", fact.Name, len(fact.FullContent))
		}
	case engine.FactSystemReminder:
		switch fact.Name {
		case string(engine.ConversationSourceReminder):
			log.Printf("[Reminder] 已注入系统提醒")
		case string(engine.ConversationSourceTODOGate):
			log.Printf("[TODO] Final response blocked until TODO.md is updated")
		}
	case engine.FactErrorRecovery:
		log.Printf("[Recovery] 已注入错误恢复提示")
	case engine.FactRunCompleted:
		log.Printf("[Engine] 模型不再需要调用工具，宣告任务完成！")
	}
	o.base.Observe(ctx, fact)
}

var _ engine.Observer = (*diagnosticObserver)(nil)
var _ engine.ModelInvoker = diagnosticModel{}
