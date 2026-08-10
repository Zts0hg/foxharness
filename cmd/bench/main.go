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
	"github.com/Zts0hg/foxharness/internal/compaction"
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
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	flags := flag.NewFlagSet("bench", flag.ContinueOnError)
	casePath := flags.String("case", "", "benchmark case yaml path")
	outPath := flags.String("out", "benchmark-result.json", "result json path")
	repeat := flags.Int("repeat", 1, "number of times to repeat the benchmark")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if *casePath == "" {
		log.Print("请通过 -case 指定 benchmark case")
		return 2
	}

	c, err := benchmark.LoadCase(*casePath)
	if err != nil {
		log.Print(err)
		return 2
	}

	runner := benchmark.NewRunner(buildHarness)
	var results []*benchmark.Result
	infrastructureFailed := false

	for i := 0; i < *repeat; i++ {
		result, err := runner.RunCase(context.Background(), c)
		if result != nil {
			results = append(results, result)
		}
		if err != nil {
			log.Print(err)
			infrastructureFailed = true
			break
		}
	}

	benchmark.PrintSummary(results)
	if err := benchmark.WriteJSON(*outPath, results); err != nil {
		log.Print(err)
		return 2
	}
	return resultExitCode(results, infrastructureFailed)
}

func resultExitCode(results []*benchmark.Result, infrastructureFailed bool) int {
	if infrastructureFailed {
		return 2
	}
	for _, result := range results {
		if result == nil || result.Status == benchmark.ResultStatusInfrastructureFailed {
			return 2
		}
		if !result.Success {
			return 1
		}
	}
	return 0
}

// buildHarness creates an AgentEngine and Session for a benchmark run.
// It sets up the LLM provider, tool registry, memory store, and session
// manager configured for the given benchmark case.
func buildHarness(ctx context.Context, workDir string, c *benchmark.Case) (*benchmark.Harness, error) {
	_ = ctx
	manager := session.NewManager(workDir)
	sess, err := manager.Create(session.CreateOptions{
		Source:  session.SOURCECLI,
		WorkDir: workDir,
	})
	if err != nil {
		return nil, err
	}

	composer := prompt.NewComposer(workDir).WithMemory(sess.MemoryPath())
	store := memory.NewSessionStore(workDir, sess.RootDir)
	if err := store.EnsureFiles(); err != nil {
		return nil, err
	}
	homeDir, _ := os.UserHomeDir()
	llmConfig, err := resolveBenchmarkLLMConfig(homeDir, os.Getenv)
	if err != nil {
		return nil, err
	}
	llmProvider, err := provider.NewProvider(llmConfig)
	if err != nil {
		return nil, err
	}
	registry := buildBenchmarkRegistry(workDir, sess)

	eng := engine.NewAgentEngine(
		llmProvider,
		registry,
		workDir,
		composer,
		engine.Config{
			MaxTurns:         c.MaxTurns,
			ProviderProtocol: llmConfig.Protocol,
			Model:            llmConfig.Model,
		},
	)
	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = llmConfig.Model
	compCfg.SessionDir = sess.RootDir
	compCfg.TranscriptPath = sess.TranscriptPath()
	compactor, err := compaction.NewCompactor(llmProvider, compCfg)
	if err != nil {
		return nil, err
	}
	eng.WithCompactor(compactor)

	return &benchmark.Harness{
		Engine:  eng,
		Session: sess,
		RuntimeFidelity: benchmark.RuntimeFidelity{
			SharedInvariants: []string{
				"todo tool surface",
				"context compaction",
				"structured tool failure semantics",
			},
			IntentionalDifferences: []string{
				"no interactive approval surface",
				"no TUI ask_user_question surface",
			},
			Warning: "benchmark runtime intentionally reports product-runtime differences",
		},
	}, nil
}

func buildBenchmarkRegistry(workDir string, sess *session.Session) tools.Registry {
	registry := tools.NewRegistry()
	registry.Register(tools.NewReadFileTool(workDir))
	registry.Register(tools.NewWriteFileTool(workDir))
	registry.Register(tools.NewBashTool(workDir))
	registry.Register(tools.NewEditFileTool(workDir))
	registry.Register(tools.NewReadTodoTool(sess.RootDir))
	registry.Register(tools.NewUpdateTodoTool(sess.RootDir))
	return registry
}

func resolveBenchmarkLLMConfig(homeDir string, lookup llmconfig.EnvLookup) (llmconfig.ResolvedConfig, error) {
	return llmresolve.FromUserSettings(homeDir, llmconfig.CLIOverrides{}, lookup)
}
