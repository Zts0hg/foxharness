package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/compaction"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
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
	composer := prompt.NewComposer(workDir).WithMemory(sess.MemoryPath())
	eng := engine.NewAgentEngine(llmProvider, registry, workDir, true, composer)
	eng.WithCompactor(compaction.NewCompactor(
		llmProvider,
		compaction.RoughEstimator{},
		// compaction.DefaultConfig(),
		compaction.Config{
			MaxTokens:        4000,
			SoftRatio:        0.5,
			RecentKeep:       2,
			SummaryMaxTokens: 1024,
		},
	))

	fmt.Println("开始执行任务...")
	prompt := `请你读取当前项目下的所有文件，帮我在项目根目录生成一份 README.md 文档`
	err = eng.Run(context.Background(), sess, prompt)
	if err != nil {
		log.Fatalf("引擎运行崩溃: %v", err)
	}
}
