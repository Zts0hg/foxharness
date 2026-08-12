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

type benchmarkOptions struct {
	casePath string
	outPath  string
	repeat   int
}

type benchmarkCommandDependencies struct {
	loadCase     func(string) (*benchmark.Case, error)
	execute      func(context.Context, *benchmark.Case, int) ([]*benchmark.Result, bool)
	printSummary func([]*benchmark.Result)
	writeJSON    func(string, []*benchmark.Result) error
}

func newBenchmarkFlagSet() (*flag.FlagSet, *benchmarkOptions) {
	flags := flag.NewFlagSet("bench", flag.ContinueOnError)
	options := &benchmarkOptions{}
	flags.StringVar(&options.casePath, "case", "", "benchmark case yaml path")
	flags.StringVar(&options.outPath, "out", "benchmark-result.json", "result json path")
	flags.IntVar(&options.repeat, "repeat", 1, "number of times to repeat the benchmark")
	return flags, options
}

func run(args []string) int {
	return runWithDependencies(args, defaultBenchmarkCommandDependencies())
}

func defaultBenchmarkCommandDependencies() benchmarkCommandDependencies {
	return benchmarkCommandDependencies{
		loadCase: benchmark.LoadCase,
		execute: func(ctx context.Context, c *benchmark.Case, repeat int) ([]*benchmark.Result, bool) {
			runner := benchmark.NewRunner(buildHarness)
			return executeRepeats(ctx, c, repeat, runner.RunRepeat)
		},
		printSummary: benchmark.PrintSummary,
		writeJSON:    benchmark.WriteJSON,
	}
}

func runWithDependencies(args []string, dependencies benchmarkCommandDependencies) int {
	flags, options := newBenchmarkFlagSet()
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		log.Print("benchmark 不接受位置参数")
		return 2
	}
	if options.repeat <= 0 {
		log.Print("-repeat 必须是正整数")
		return 2
	}

	if options.casePath == "" {
		log.Print("请通过 -case 指定 benchmark case")
		return 2
	}

	c, err := dependencies.loadCase(options.casePath)
	if err != nil {
		log.Print(err)
		return 2
	}

	results, infrastructureFailed := dependencies.execute(context.Background(), c, options.repeat)

	dependencies.printSummary(results)
	if err := dependencies.writeJSON(options.outPath, results); err != nil {
		log.Print(err)
		return 2
	}
	return resultExitCode(results, infrastructureFailed)
}

type repeatExecutor func(context.Context, *benchmark.Case, int) (*benchmark.Result, error)

func executeRepeats(ctx context.Context, c *benchmark.Case, repeat int, execute repeatExecutor) ([]*benchmark.Result, bool) {
	var results []*benchmark.Result
	for index := 1; index <= repeat; index++ {
		result, err := execute(ctx, c, index)
		if result != nil {
			results = append(results, result)
		}
		if err != nil {
			log.Print(err)
			return results, true
		}
	}
	return results, false
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
	toolNames := make([]string, 0)
	for _, definition := range registry.GetAvailableTools() {
		toolNames = append(toolNames, definition.Name)
	}
	runtimeSpec := benchmark.NewRuntimeSpec(llmConfig.Protocol, llmConfig.Model, c.MaxTurns, toolNames)

	eng := engine.NewAgentEngine(
		llmProvider,
		registry,
		workDir,
		composer,
		engine.Config{
			MaxTurns:         runtimeSpec.MaxTurns,
			ProviderProtocol: runtimeSpec.ProviderProtocol,
			Model:            runtimeSpec.Model,
		},
	)
	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = runtimeSpec.Model
	compCfg.SessionDir = sess.RootDir
	compCfg.TranscriptPath = sess.TranscriptPath()
	compactor, err := compaction.NewCompactor(llmProvider, compCfg)
	if err != nil {
		return nil, err
	}
	eng.WithCompactor(compactor)

	return &benchmark.Harness{
		Engine:          eng,
		Session:         sess,
		RuntimeFidelity: runtimeSpec.Fidelity(),
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
