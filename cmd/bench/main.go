package main

import (
	"context"
	"flag"
	"log"

	"github.com/Zts0hg/foxharness/internal/benchmark"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func main() {
	casePath := flag.String("case", "", "benchmark case yaml path")
	outPath := flag.String("out", "benchmark-result.json", "result json path")
	repeat := flag.Int("repeat", 1, "repeat count")
	flag.Parse()

	if *casePath == "" {
		log.Fatal("请通过 -case 指定 benchmark case")
	}

	c, err := benchmark.LoadCase(*casePath)
	if err != nil {
		log.Fatal(err)
	}

	runner := benchmark.NewRunner(buildHarness)
	var results []*benchmark.Result

	for i := 0; i < *repeat; i++ {
		result, err := runner.RunCase(context.Background(), c)
		if err != nil {
			log.Fatal(err)
		}
		results = append(results, result)
	}

	benchmark.PrintSummary(results)
	if err := benchmark.WriteJSON(*outPath, results); err != nil {
		log.Fatal(err)
	}
}

func buildHarness(ctx context.Context, workDir string, c *benchmark.Case) (*engine.AgentEngine, *session.Session, error) {
	manager := session.NewManager(workDir)
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		return nil, nil, err
	}

	store := memory.NewStore(workDir)
	if err := store.EnsureFiles(); err != nil {
		return nil, nil, err
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	composer := prompt.NewComposer(workDir).WithMemory(sess.MemoryPath())
	llmProvider := provider.NewZhipuOpenAIProvider("glm-4.5-air")

	enableThinking := false
	planner := memory.NewPlanner(llmProvider, store)
	if err := planner.BuildPlan(ctx, c.Prompt); err != nil {
		log.Printf("[PlanMode] 生成计划失败，将回退到旧版每轮 Thinking: %v", err)
		enableThinking = true
	} else {
		log.Printf("[PlanMode] 计划已生成，本次 Benchmark Run 关闭每轮 Thinking")
	}

	eng := engine.NewAgentEngine(
		llmProvider,
		registry,
		workDir,
		composer,
		engine.Config{
			EnableThinking: enableThinking,
			MaxTurns:       c.MaxTurns,
		},
	)

	return eng, sess, nil
}
