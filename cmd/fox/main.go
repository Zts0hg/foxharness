package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type mockProvider struct {
	turn int
}

func (m *mockProvider) Generate(ctx context.Context, msgs []schema.Message, _ []schema.ToolDefinition) (*schema.Message, error) {
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

func (m *mockRegistry) GetAvailableTools() []schema.ToolDefinition { return nil }

func (m *mockRegistry) Execute(ctx context.Context, call schema.ToolCall) schema.ToolReuslt {
	return schema.ToolReuslt{
		ToolCallID: call.ID,
		Output:     "-rw-r--r-- 1 user group 234 Oct 24 10:00 main.go\n",
		IsError:    false,
	}
}

func main() {
	fmt.Println("🚀 欢迎来到 fox-harness-go 引擎启动序列")
	// TODO 1. 初始化大模型 Provider (大脑)
	// provider := provider.NewClaudeProvider(...)
	workDir, _ := os.Getwd()
	p := &mockProvider{}
	r := &mockRegistry{}

	eng := engine.NewAgentEngine(p, r, workDir)
	err := eng.Run(context.Background(), "帮我检查当前目录的文件")
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

	log.Println("架构蓝图搭建完毕，等待各个核心模块注入！")
}
