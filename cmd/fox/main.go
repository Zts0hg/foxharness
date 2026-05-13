package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/Zts0hg/foxharness/internal/app"
)

func main() {
	var cfg app.CLIConfig
	flag.StringVar(&cfg.WorkDir, "workdir", ".", "工作区目录")
	flag.StringVar(&cfg.Prompt, "prompt", "", "用户任务；为空时从 stdin 读取")
	flag.StringVar(&cfg.Model, "model", "glm-4.5-air", "模型名称")
	flag.BoolVar(&cfg.EnableThinking, "thinking", false, "是否启用旧版每轮 Thinking；Plan Mode 成功时会关闭它")
	flag.BoolVar(&cfg.EnablePlanMode, "plan", true, "是否启用 Plan Mode")
	flag.IntVar(&cfg.MaxTurns, "max-turns", 20, "最大 Turn 数")
	flag.Parse()

	prompt, err := readPrompt(cfg.Prompt)
	if err != nil {
		log.Fatal(err)
	}

	cfg.Prompt = prompt
	if err := app.RunCLI(context.Background(), cfg); err != nil {
		log.Fatal(err)
	}

	// 	if os.Getenv("ZHIPU_API_KEY") == "" {
	// 		panic("请先导出 ZHIPU_API_KEY 环境变量")
	// 	}

	// 	fmt.Println("🚀 欢迎来到 fox-harness-go 引擎启动序列")

	// 	workDir, _ := os.Getwd()
	// 	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.7")
	// 	registry := tools.NewRegistry()

	// 	registry.Register(tools.NewReadFileTool(workDir))
	// 	registry.Register(tools.NewWriteFileTool(workDir))
	// 	registry.Register(tools.NewBashTool(workDir))
	// 	registry.Register(tools.NewEditFileTool(workDir))

	// 	// TODO 3. 初始化上下文管理器 (内存管理器)
	// 	// ctxManager := context.NewManager(...)
	// 	manager := session.NewManager(workDir)
	// 	sess, err := manager.Create(session.CreateOptions{
	// 		Source:  session.SOURCECLI,
	// 		WorkDir: workDir,
	// 	})
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	subManager := subagent.NewManager(llmProvider, workDir)
	// 	registry.Register(subagent.NewTool(subManager, sess.ID))

	// 	store := memory.NewStore(workDir)
	// 	if err := store.EnsureFiles(); err != nil {
	// 		log.Fatal(err)
	// 	}

	// 	userPrompt := `请严格按顺序执行这个验证任务：

	// 1. 第一步必须调用 read_file，读取 ./DOES_NOT_EXIST_FOR_TRACE.md。
	// 2. 这个文件不存在。读取失败后，等待 Harness 的 Error Recovery Notice。
	// 3. 收到恢复提示后，使用 bash 查看当前目录。
	// 4. 然后读取 go.mod，总结 module 名称和 Go 版本。`
	// 	enablePlanMode := true
	// 	enableThinking := false
	// 	if enablePlanMode {
	// 		planner := memory.NewPlanner(llmProvider, store)
	// 		log.Printf("[PlanMode] 开始生成计划...")
	// 		if err := planner.BuildPlan(context.Background(), userPrompt); err != nil {
	// 			log.Printf("[PlanMode] 生成计划失败，将回退到旧版每轮 Thinking: %v", err)
	// 			enableThinking = true
	// 		} else {
	// 			log.Printf("[PlanMode] 计划已生成，本次任务关闭每轮 Thinking")

	// 		}

	// 	}

	// 	composer := prompt.NewComposer(workDir).WithMemory(sess.MemoryPath())
	// 	eng := engine.NewAgentEngine(llmProvider, registry, workDir, enableThinking, composer)
	// 	eng.WithCompactor(compaction.NewCompactor(
	// 		llmProvider,
	// 		compaction.RoughEstimator{},
	// 		compaction.DefaultConfig(),
	// 	))

	// fmt.Println("开始执行任务...")
	// _, err = eng.Run(context.Background(), sess, userPrompt)
	//
	//	if err != nil {
	//		log.Fatalf("引擎运行崩溃: %v", err)
	//	}
}

func readPrompt(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input != "" {
		return input, nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", fmt.Errorf("prompt 不能为空，请使用 -prompt 或通过 stdin 输入")

	}

	return prompt, nil
}
