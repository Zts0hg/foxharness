package benchmark

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/modelinvoke"
	"github.com/Zts0hg/foxharness/internal/prompt"
	"github.com/Zts0hg/foxharness/internal/provider"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/toolexec"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/turnpolicy"
)

func NewRuntimeSpec(protocol, model string, maxTurns int, toolNames []string) BenchmarkRuntimeSpec {
	requestedTools := append([]string{}, toolNames...)
	runSpec := foxruntime.RunSpec{
		ProviderProtocol: protocol, Model: model, MaxTurns: &maxTurns, AllowedTools: requestedTools,
	}
	spec, err := ResolveRuntimeSpec(runSpec)
	if err != nil {
		panic(err)
	}
	return spec
}

type benchmarkComposerFactory func(foxruntime.RunAssembly) engine.PromptComposer
type benchmarkRegistryFactory func(foxruntime.RunAssembly) tools.Registry

func newTargetBenchmarkHarness(
	ctx context.Context,
	workDir string,
	home string,
	spec BenchmarkRuntimeSpec,
	model provider.LLMProvider,
	composerFactory benchmarkComposerFactory,
	registryFactory benchmarkRegistryFactory,
) (*Harness, error) {
	store := session.NewFileStoreWithHome(workDir, home)
	return newTargetBenchmarkHarnessWithStore(ctx, workDir, store, spec, model, composerFactory, registryFactory)
}

func newTargetBenchmarkHarnessWithStore(
	ctx context.Context,
	workDir string,
	store foxruntime.SessionStore,
	spec BenchmarkRuntimeSpec,
	model provider.LLMProvider,
	composerFactory benchmarkComposerFactory,
	registryFactory benchmarkRegistryFactory,
) (*Harness, error) {
	dependencies := foxruntime.HarnessDependencies{
		NewModel: func(context.Context, foxruntime.RunAssembly) (engine.ModelInvoker, error) {
			return modelinvoke.New(model, modelinvoke.Config{}), nil
		},
		NewTools: func(_ context.Context, assembly foxruntime.RunAssembly) (engine.ToolExecutor, error) {
			if registryFactory == nil {
				return toolruntime.New(nil, toolresult.OSFileSystem{}, filepath.Join(assembly.Session.RootDir, "tool-results")), nil
			}
			return toolruntime.New(testCapabilities(registryFactory(assembly), assembly.AllowedTools), toolresult.OSFileSystem{}, filepath.Join(assembly.Session.RootDir, "tool-results")), nil
		},
		NewPolicy: func(context.Context, foxruntime.RunAssembly) (engine.TurnPolicy, error) {
			return turnpolicy.New(turnpolicy.Config{}), nil
		},
		NewContext: func(_ context.Context, assembly foxruntime.RunAssembly) (foxruntime.ContextCollector, foxruntime.ContextCompactor, error) {
			return benchmarkContextCollector{composer: composerFactory(assembly)}, nil, nil
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
	maxTurns := spec.MaxTurns
	runSpec := foxruntime.RunSpec{
		ProviderProtocol: spec.ProviderProtocol,
		Model:            spec.Model,
		MaxTurns:         &maxTurns,
		AllowedTools:     append([]string{}, spec.ToolSurface...),
		WorkDir:          workDir,
	}
	return &Harness{Session: agentSession, RunSpec: runSpec, RuntimeFidelity: spec.Fidelity()}, nil
}

type benchmarkContextCollector struct {
	composer engine.PromptComposer
}

func (c benchmarkContextCollector) Collect(_ context.Context, request foxruntime.ContextCollectionRequest) ([]prompt.Fragment, error) {
	text, err := c.composer.Compose(request.Prompt)
	if err != nil {
		return nil, fmt.Errorf("组装系统提示词失败: %w", err)
	}
	return []prompt.Fragment{prompt.Text(text)}, nil
}

func testCapabilities(registry tools.Registry, allowedTools []string) []toolexec.Capability {
	allowed := make(map[string]struct{}, len(allowedTools))
	for _, name := range allowedTools {
		allowed[name] = struct{}{}
	}
	var capabilities []toolexec.Capability
	for _, definition := range registry.GetAvailableTools() {
		if _, ok := allowed[definition.Name]; !ok {
			continue
		}
		definition := definition
		capabilities = append(capabilities, toolexec.Capability{
			Definition: definition, ParallelSafe: registry.IsParallelSafe(definition.Name),
			Execute: func(ctx context.Context, call engine.ToolCall) engine.ToolExecutionResult {
				result := registry.Execute(ctx, call)
				return engine.ToolExecutionResult{
					CallID: result.ToolCallID, FullContent: result.Output,
					ModelContent: result.Output, ObserverContent: result.Output, IsError: result.IsError,
				}
			},
		})
	}
	return capabilities
}
