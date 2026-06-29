// Package main is the entry point for the benchmark runner.
//
// The benchmark runner executes agent tasks defined in YAML case files
// and validates the results against expected outcomes.
//
// Usage:
//
//	go run cmd/bench/main.go -case benchmarks/fixtures/counter_race/case.yaml
//	go run cmd/bench/main.go -case case.yaml -out results.json -repeat 3
//
// Flags:
//
//	-case   Path to the benchmark case YAML file (required)
//	-out    Path for the JSON results file (default: "benchmark-result.json")
//	-repeat Number of times to repeat the benchmark (default: 1)
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/Zts0hg/foxharness/internal/benchmark"
	prompt "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/llmresolve"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func main() {
	casePath := flag.String("case", "", "benchmark case yaml path")
	outPath := flag.String("out", "benchmark-result.json", "result json path")
	repeat := flag.Int("repeat", 1, "number of times to repeat the benchmark")
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

// buildHarness creates an AgentEngine and Session for a benchmark run.
// It sets up the LLM provider, tool registry, memory store, and session
// manager configured for the given benchmark case.
func buildHarness(ctx context.Context, workDir string, c *benchmark.Case) (*engine.AgentEngine, *session.Session, error) {
	manager := session.NewManager(workDir)
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		return nil, nil, err
	}

	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))

	composer := prompt.NewComposer(workDir).WithMemory(sess.MemoryPath())
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		return nil, nil, err
	}
	homeDir, _ := os.UserHomeDir()
	llmConfig, err := resolveBenchmarkLLMConfig(homeDir, os.Getenv)
	if err != nil {
		return nil, nil, err
	}
	llmProvider, err := provider.NewProvider(llmConfig)
	if err != nil {
		return nil, nil, err
	}

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
			EnableThinking:   enableThinking,
			MaxTurns:         c.MaxTurns,
			ProviderProtocol: llmConfig.Protocol,
			Model:            llmConfig.Model,
		},
	)

	return eng, sess, nil
}

func resolveBenchmarkLLMConfig(homeDir string, lookup llmconfig.EnvLookup) (llmconfig.ResolvedConfig, error) {
	return llmresolve.FromUserSettings(homeDir, llmconfig.CLIOverrides{}, lookup)
}
