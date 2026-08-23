package registryexec

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolexec"
	"github.com/Zts0hg/foxharness/internal/toolprotocol"
	"github.com/Zts0hg/foxharness/internal/tools"
)

/* ResultHook observes one completed concrete registry execution. */
type ResultHook func(schema.ToolCall, schema.ToolResult)

/* ContextHook enriches one execution context before the concrete registry receives it. */
type ContextHook func(context.Context) context.Context

/* Capabilities freezes the allowed advertised and executable registry surface. */
func Capabilities(registry tools.Registry, allowedNames []string, hook ResultHook) []toolexec.Capability {
	return CapabilitiesWithContext(registry, allowedNames, nil, hook)
}

/* CapabilitiesWithContext also applies run-scoped context before concrete execution. */
func CapabilitiesWithContext(registry tools.Registry, allowedNames []string, contextHook ContextHook, resultHook ResultHook) []toolexec.Capability {
	allowed := make(map[string]struct{}, len(allowedNames))
	for _, name := range allowedNames {
		allowed[name] = struct{}{}
	}
	definitions := make([]schema.ToolDefinition, 0, len(allowedNames))
	for _, available := range registry.GetAvailableTools() {
		if _, ok := allowed[available.Name]; !ok {
			continue
		}
		definitions = append(definitions, available)
	}
	allowedSnapshot := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		allowedSnapshot = append(allowedSnapshot, definition.Name)
	}
	capabilities := make([]toolexec.Capability, 0, len(definitions))
	for _, definition := range definitions {
		capabilities = append(capabilities, toolexec.Capability{
			Definition: definition, ParallelSafe: registry.IsParallelSafe(definition.Name),
			Execute: func(ctx context.Context, call schema.ToolCall) engine.ToolExecutionResult {
				if contextHook != nil {
					ctx = contextHook(ctx)
				}
				ctx = toolprotocol.WithCapabilities(ctx, allowedSnapshot)
				executed := registry.Execute(ctx, call)
				if resultHook != nil {
					copy := call
					copy.Arguments = append([]byte(nil), call.Arguments...)
					resultHook(copy, executed)
				}
				return engine.ToolExecutionResult{
					CallID: executed.ToolCallID, FullContent: executed.Output,
					ModelContent: executed.Output, ObserverContent: executed.Output, IsError: executed.IsError,
				}
			},
		})
	}
	return capabilities
}
