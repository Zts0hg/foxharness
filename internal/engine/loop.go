package engine

import (
	"context"
	"fmt"
	"log"

	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type AgentEngine struct {
	provider       provider.LLMProvider
	registry       tools.Registry
	workDir        string
	enableThinking bool
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string, enableThinking bool) *AgentEngine {
	return &AgentEngine{
		provider:       p,
		registry:       r,
		workDir:        workDir,
		enableThinking: enableThinking,
	}
}

func (e *AgentEngine) Run(ctx context.Context, userPrompt string) error {
	log.Printf("[Engine] 引擎启动，锁定工作区：%s\n", e.workDir)
	log.Printf("[Engine] 慢思考模式（Thinking Phase）: %v\n", e.enableThinking)

	contextHistory := []schema.Message{
		{
			Role:    schema.RoleSystem,
			Content: "You are fox-harness-go, an expert coding assistant. You have full access to tools in the workspace.",
		},
		{
			Role:    schema.RoleUser,
			Content: userPrompt,
		},
	}

	turnCount := 0

	for {
		turnCount++
		log.Printf("====== [Turn %d] 开始", turnCount)

		availableTools := e.registry.GetAvailableTools()
		if e.enableThinking {
			log.Println("[Engine][Phase 1] 剥夺工具访问权，强制进入慢思考与规划阶段...")
			thinkingResponse, err := e.provider.Generate(ctx, contextHistory, nil)
			if err != nil {
				return fmt.Errorf("Thinking 阶段生成失败: %w", err)
			}

			if thinkingResponse.Content != "" {
				log.Printf("🧠 [内部思考 Trace]: %s\n", thinkingResponse.Content)
			}

		}
		log.Println("[Engine][Phase 2] 恢复工具挂载，等待模型采取行动...")
		actionResponse, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("模型生成失败: %w", err)
		}
		contextHistory = append(contextHistory, *actionResponse)

		if actionResponse.Content != "" {
			fmt.Printf("🤖 [对外回复]: %s\n", actionResponse.Content)
		}

		if len(actionResponse.ToolCalls) == 0 {
			log.Printf("[Engine] 模型不再需要调用工具，宣告任务完成！")
			break
		}

		log.Printf("[Engine] 模型请求调用 %d 个工具\n", len(actionResponse.ToolCalls))

		for _, toolCall := range actionResponse.ToolCalls {
			log.Printf("执行工具: %s, 参数: %s\n", toolCall.Name, toolCall.Arguments)

			result := e.registry.Execute(ctx, toolCall)
			if result.IsError {
				log.Printf("  -> ❌ 工具执行报错: %s\n", result.Output)
			} else {
				log.Printf("  -> ✅ 工具执行成功（返回 %d 字节）\n", len(result.Output))
			}

			observationMessage := schema.Message{
				Role:       schema.RoleAssistant,
				Content:    result.Output,
				ToolCallID: toolCall.ID,
			}

			contextHistory = append(contextHistory, observationMessage)
		}
	}

	return nil
}
