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
	provider provider.LLMProvider
	registry tools.Registry
	workDir  string
}

func NewAgentEngine(p provider.LLMProvider, r tools.Registry, workDir string) *AgentEngine {
	return &AgentEngine{
		provider: p,
		registry: r,
		workDir:  workDir,
	}
}

func (e *AgentEngine) Run(ctx context.Context, userPrompt string) error {
	log.Printf("[Engine] 引擎启动，锁定工作区：%s\n", e.workDir)

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

		log.Println("[Engine] 正在思考...")
		responseMessage, err := e.provider.Generate(ctx, contextHistory, availableTools)
		if err != nil {
			return fmt.Errorf("模型生成失败: %w", err)
		}
		contextHistory = append(contextHistory, *responseMessage)

		if responseMessage.Content != "" {
			fmt.Printf("🤖 模型: %s\n", responseMessage.Content)
		}

		if len(responseMessage.ToolCalls) == 0 {
			log.Printf("[Engine] 任务完成，退出循环")
			break
		}

		log.Printf("[Engine] 模型请求调用 %d 个工具\n", len(responseMessage.ToolCalls))

		for _, toolCall := range responseMessage.ToolCalls {
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
