package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type mockProvider struct {
	turn int
}

func (m *mockProvider) Generate(ctx context.Context, msgs []schema.Message, tools []schema.ToolDefinition) (*schema.Message, error) {
	if len(tools) == 0 {
		return &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "【推理中】目标是检索文件。我不能直接盲猜，我需要先调用 bash 工具执行 ls 命令，看看当前目录下有什么，然后再做定夺",
		}, nil

	}

	m.turn++
	if m.turn == 1 {
		return &schema.Message{
			Role:    schema.RoleAssistant,
			Content: "让我来看看当前目录下有什么文件。",
			ToolCalls: []schema.ToolCall{
				{ID: "call_123", Name: "bash", Arguments: []byte(`{"commands": "ls -la"}`)},
			},
		}, nil
	}

	return &schema.Message{
		Role:    schema.RoleAssistant,
		Content: "我看到了文件列表，里面包含 main.go，任务完成！",
	}, nil
}

type mockRegistry struct{}

func (m *mockRegistry) GetAvailableTools() []schema.ToolDefinition {
	return []schema.ToolDefinition{
		{
			Name:        "get_weather",
			Description: "获取指定城市的当前天气情况。",
			InputScheme: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []string{"city"},
			},
		},
	}
}

func (m *mockRegistry) Execute(ctx context.Context, call schema.ToolCall) schema.ToolReuslt {
	log.Printf("  -> [Mock 工具执行] 获取 %s 的天气中...\n", call.Name)
	return schema.ToolReuslt{
		ToolCallID: call.ID,
		Output:     "API 返回: 今天是晴天，气温 25 度。",
		IsError:    false,
	}
}

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		panic("请先导出 ZHIPU_API_KEY 环境变量")
	}

	fmt.Println("🚀 欢迎来到 fox-harness-go 引擎启动序列")

	// TODO 1. 初始化大模型 Provider (大脑)
	// provider := provider.NewClaudeProvider(...)
	workDir, _ := os.Getwd()
	p := provider.NewZhipuOpenAIProvider("glm-4.5-air")
	r := &mockRegistry{}

	eng := engine.NewAgentEngine(p, r, workDir, true)

	prompt := "我想去北京跑步，帮我查查天气合适吗？"
	err := eng.Run(context.Background(), prompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}

	// TODO 2. 初始化 Tool Registry (手脚)
	// registry := tools.NewRegistry()
	// registry.Registry(tools.NewBashTool())

	// TODO 3. 初始化上下文管理器 (内存管理器)
	// ctxManager := context.NewManager(...)

	// TODO 4. 组装并启动核心 Engine (操作系统心脏)
	// engine := engine.NewAgentEngine(provider, registry, ctxManager)

	// fmt.Println("开始执行任务...")
	// err := engine.Run("帮我检查一下当前目录下的文件，并输出一个 README.md 大纲")
	// if err != nil {
	// 	log.Fatalf("引擎运行崩溃: %v", err)
	// }

	// log.Println("架构蓝图搭建完毕，等待各个核心模块注入！")
}
