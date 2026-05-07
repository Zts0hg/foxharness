package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func main() {
	if os.Getenv("ZHIPU_API_KEY") == "" {
		panic("请先导出 ZHIPU_API_KEY 环境变量")
	}

	fmt.Println("🚀 欢迎来到 fox-harness-go 引擎启动序列")

	workDir, _ := os.Getwd()
	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.7")
	registry := tools.NewRegistry()

	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	// TODO 3. 初始化上下文管理器 (内存管理器)
	// ctxManager := context.NewManager(...)
	manager := session.NewManager(workDir)
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		log.Fatal(err)
	}
	subManager := subagent.NewManager(llmProvider, workDir)
	registry.Register(subagent.NewTool(subManager, sess.ID))

	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		log.Fatal(err)
	}

	userPrompt := `请严格按顺序执行这个验证任务：

1. 第一步必须调用 delegate_task。
2. 委派任务内容是：只读探索当前项目的工具系统，重点查看 internal/tools 和 go.mod，总结已经实现了哪些工具、模块名是什么、工具注册入口在哪里。
3. delegate_task 必须设置 read_only=true。
4. 收到 Subagent 报告后，主 Agent 再用不超过 300 字总结报告结论，不要自己重新读取所有文件。`
	enablePlanMode := true
	enableThinking := false
	if enablePlanMode {
		planner := memory.NewPlanner(llmProvider, store)
		log.Printf("[PlanMode] 开始生成计划...")
		if err := planner.BuildPlan(context.Background(), userPrompt); err != nil {
			log.Printf("[PlanMode] 生成计划失败，将回退到旧版每轮 Thinking: %v", err)
			enableThinking = true
		} else {
			log.Printf("[PlanMode] 计划已生成，本次任务关闭每轮 Thinking")

		}

	}

	composer := prompt.NewComposer(workDir).WithMemory(sess.MemoryPath())
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, enableThinking, composer)
	eng.WithCompactor(compaction.NewCompactor(
		llmProvider,
		compaction.RoughEstimator{},
		compaction.DefaultConfig(),
	))

	fmt.Println("开始执行任务...")
	_, err = eng.Run(context.Background(), sess, userPrompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}
