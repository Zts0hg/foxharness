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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Zts0hg/foxharness/internal/benchmark"
	"github.com/Zts0hg/foxharness/internal/compaction"
	legacycontext "github.com/Zts0hg/foxharness/internal/context"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/llmresolve"
	"github.com/Zts0hg/foxharness/internal/memory"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/registryexec"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/runtimecompaction"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/turnpolicy"
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

/* buildHarness composes one isolated BenchmarkEval runtime session. */
func buildHarness(ctx context.Context, workDir string, c *benchmark.Case) (*benchmark.Harness, error) {
	store := session.NewFileStore(workDir)
	var llmProvider provider.LLMProvider
	var compactor *compaction.Compactor
	dependencies := foxruntime.HarnessDependencies{
		InitializeSession: func(_ context.Context, snapshot foxruntime.AgentSessionSnapshot) error {
			return memory.NewSessionStore(workDir, snapshot.RootDir).EnsureFiles()
		},
		NewModel: func(context.Context, foxruntime.RunAssembly) (engine.ModelInvoker, error) {
			return modelinvoke.New(llmProvider, modelinvoke.Config{OnSuccess: compactor.ResetCircuitBreaker}), nil
		},
		NewTools: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ToolExecutor, error) {
			registry := buildBenchmarkRegistry(workDir, &session.StoredSession{RootDir: assembly.Session.RootDir})
			return toolruntime.New(
				registryexec.Capabilities(registry, assembly.AllowedTools, nil),
				toolresult.OSFileSystem{}, filepath.Join(assembly.Session.RootDir, "tool-results"),
			), nil
		},
		NewPolicy: func(context.Context, foxruntime.RunAssembly) (engine.TurnPolicy, error) {
			return turnpolicy.New(turnpolicy.Config{}), nil
		},
		NewContext: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
			workingMemory := memory.NewSessionStore(workDir, assembly.Session.RootDir).WorkingMemoryPath()
			collector := benchmarkPromptCollector{composer: legacycontext.NewComposer(workDir).WithMemory(workingMemory)}
			return collector, runtimecompaction.New(compactor), nil
		},
	}
	runtimeHarness, err := foxruntime.NewRuntimeHarness(store, dependencies)
	if err != nil {
		return nil, err
	}
	agentSession, err := runtimeHarness.CreateSession(ctx, foxruntime.BenchmarkEval, foxruntime.SessionOptions{WorkDir: workDir})
	if err != nil {
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = agentSession.Close(context.Background())
		}
	}()

	homeDir, _ := os.UserHomeDir()
	llmConfig, err := resolveBenchmarkLLMConfig(homeDir, os.Getenv)
	if err != nil {
		return nil, err
	}
	llmProvider, err = provider.NewProvider(llmConfig)
	if err != nil {
		return nil, err
	}
	maxTurns := c.MaxTurns
	runSpec := foxruntime.RunSpec{
		Prompt: c.Prompt, ProviderProtocol: llmConfig.Protocol, Model: llmConfig.Model,
		WorkDir: workDir, BenchmarkCase: c.ID, MaxTurns: &maxTurns,
	}
	if c.TimeoutSeconds > 0 {
		taskTimeout := time.Duration(c.TimeoutSeconds) * time.Second
		runSpec.TaskTimeout = &taskTimeout
	}
	runtimeSpec, err := benchmark.ResolveRuntimeSpec(runSpec)
	if err != nil {
		return nil, err
	}
	compCfg := compaction.DefaultCompactionConfig()
	compCfg.Model = runtimeSpec.Model
	compCfg.SessionDir = agentSession.Snapshot().RootDir
	compCfg.TranscriptPath = filepath.Join(agentSession.Snapshot().RootDir, "transcript.jsonl")
	compactor, err = compaction.NewCompactor(llmProvider, compCfg)
	if err != nil {
		return nil, err
	}
	failed = false
	return &benchmark.Harness{
		Session:         agentSession,
		RunSpec:         runSpec,
		RuntimeFidelity: runtimeSpec.Fidelity(),
	}, nil
}

type benchmarkPromptCollector struct {
	composer benchmarkPromptComposer
}

type benchmarkPromptComposer interface {
	Compose(string) (string, error)
}

func (c benchmarkPromptCollector) Collect(_ context.Context, request foxruntime.ContextCollectionRequest) ([]prompt.Fragment, error) {
	text, err := c.composer.Compose(request.Prompt)
	if err != nil {
		return nil, fmt.Errorf("组装系统提示词失败: %w", err)
	}
	return []prompt.Fragment{prompt.Text(text)}, nil
}

func buildBenchmarkRegistry(workDir string, sess *session.StoredSession) tools.Registry {
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
