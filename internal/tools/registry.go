// Package tools provides a tool registry and built-in tool implementations for the foxharness agent.
//
// The tool system allows the LLM agent to extend its capabilities by invoking
// external functions. Tools can perform file operations, execute shell commands,
// and interact with the system in a controlled manner.
//
// Key Components:
//   - Registry: Interface for registering and executing tools with middleware support
//   - BaseTool: Core interface that all tools must implement
//   - ParallelSafeTool: Optional interface for marking tools safe for parallel execution
//
// Built-in Tools:
//   - bash: Execute shell commands
//   - read_file: Read file contents
//   - write_file: Create or overwrite files
//   - edit_file: Make targeted edits to files
//
// Middleware Support:
// Tools can be wrapped with middleware for approval workflows, safety checks,
// and other cross-cutting concerns.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// Registry defines the interface for tool registration and execution.
// It manages the lifecycle of tools, executes tool calls, and provides
// tool discovery for the LLM.
type Registry interface {
	// Register adds a tool to the registry. If a tool with the same name
	// already exists, it will be overwritten.
	Register(tool BaseTool)

	// Use adds a middleware that will be invoked before each tool execution.
	// Multiple middlewares can be added; they are executed in registration order.
	Use(middleware middleware.Middleware)

	// GetAvailableTools returns definitions for all registered tools.
	// This is used to inform the LLM about available capabilities.
	GetAvailableTools() []schema.ToolDefinition

	// Execute invokes a tool by name with the provided arguments.
	// All registered middlewares are invoked before the tool execution.
	// Returns the tool's output or an error if execution fails.
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult

	// IsParallelSafe reports whether a tool can be executed in parallel
	// with other tools. Tools must explicitly implement ParallelSafeTool
	// and return true to be considered parallel-safe.
	IsParallelSafe(toolName string) bool
}

// TurnAwareRegistry performs an optional lifecycle transition immediately
// before the engine discovers tools for a new model turn.
type TurnAwareRegistry interface {
	BeginTurn()
}

// BaseTool defines the core interface that all tools must implement.
// Tools provide name, definition, and execution logic.
type BaseTool interface {
	// Name returns the unique identifier for this tool.
	// This name is used to invoke the tool and must match the
	// name in the tool definition.
	Name() string

	// Definition returns the tool's schema including name, description,
	// and input schema. This is sent to the LLM to describe the tool's usage.
	Definition() schema.ToolDefinition

	// Execute runs the tool with the provided arguments.
	// The args parameter contains the JSON-encoded tool arguments.
	// Returns the tool's output as a string, or an error if execution fails.
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

// ParallelSafeTool is an optional interface that tools can implement
// to indicate they are safe for parallel execution with other tools.
// Only tools implementing this interface and returning true from ParallelSafe
// will be executed in parallel batches.
type ParallelSafeTool interface {
	// ParallelSafe reports whether this tool can be executed concurrently
	// with other parallel-safe tools. Returns true if safe, false otherwise.
	ParallelSafe() bool
}

// AliasableTool is an optional interface that tools implement to advertise
// additional names under which the same tool is callable.
//
// Imported command and skill prompts may reference a tool by a name that
// differs from this harness's canonical one — for example the externally
// sourced CodexSpec commands call the interactive question tool
// "AskUserQuestion" (the upstream naming) while fox registers it as
// "ask_user_question". Editing those prompts is not durable because they are
// re-imported on update, so the registry resolves aliases instead: it
// advertises one tool definition per alias (identical schema, the alias as the
// name) so the model sees each alias as a callable tool, and it routes an
// aliased call back to the canonical implementation.
type AliasableTool interface {
	// Aliases returns the additional call names for this tool, excluding the
	// canonical Name(). An alias equal to the canonical name is ignored.
	Aliases() []string
}

type registryImpl struct {
	tools       map[string]BaseTool
	aliases     map[string]string
	middlewares []middleware.Middleware
}

// Use adds a middleware to the registry's middleware chain.
// Middlewares are invoked in the order they were added during tool execution.
func (r *registryImpl) Use(m middleware.Middleware) {
	r.middlewares = append(r.middlewares, m)
}

// NewRegistry creates a new empty tool registry.
// Returns a Registry ready for tool registration.
func NewRegistry() Registry {
	return &registryImpl{
		tools:   make(map[string]BaseTool),
		aliases: make(map[string]string),
	}
}

// resolve maps a call name to the canonical tool name, following any alias
// registered by the tool. Unknown names are returned unchanged so the caller
// surfaces a normal "tool does not exist" error.
func (r *registryImpl) resolve(name string) string {
	if canonical, ok := r.aliases[name]; ok {
		return canonical
	}
	return name
}

// Register adds a tool to the registry. If a tool with the same name
// already exists, it logs a warning and overwrites the previous registration.
// When the tool implements AliasableTool, each alias is recorded so the tool is
// callable under those names too.
func (r *registryImpl) Register(tool BaseTool) {
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		log.Printf("[Warning] tool '%s' already registered, will be overwritten\n", name)
	}

	r.tools[name] = tool
	log.Printf("[Registry] successfully mounted tool: %s\n", name)

	if aliasable, ok := tool.(AliasableTool); ok {
		for _, alias := range aliasable.Aliases() {
			if alias == "" || alias == name {
				continue
			}
			if existing, ok := r.aliases[alias]; ok && existing != name {
				log.Printf("[Warning] alias '%s' already maps to '%s', remapping to '%s'\n", alias, existing, name)
			}
			r.aliases[alias] = name
			log.Printf("[Registry] mounted alias: %s -> %s\n", alias, name)
		}
	}
}

// GetAvailableTools returns the tool definitions for all registered tools.
// This is used to inform the LLM about the available tool capabilities. Each
// alias is advertised as its own definition (same schema, the alias as the name)
// so the model can call a tool by any name an imported prompt may reference.
func (r *registryImpl) GetAvailableTools() []schema.ToolDefinition {
	var defs []schema.ToolDefinition
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	for alias, canonical := range r.aliases {
		tool, ok := r.tools[canonical]
		if !ok {
			continue
		}
		aliasDef := tool.Definition()
		aliasDef.Name = alias
		defs = append(defs, aliasDef)
	}
	return defs
}

// Execute invokes a tool by name with the provided arguments.
// All registered middlewares are invoked before the tool execution.
// If the tool doesn't exist, or any middleware denies execution,
// or the tool execution fails, an error result is returned. The call name may
// be either the canonical tool name or any registered alias.
func (r *registryImpl) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	tool, exists := r.tools[r.resolve(call.Name)]
	if !exists {
		errMsg := fmt.Sprintf("Error: tool '%s' does not exist in the system", call.Name)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}

	for _, m := range r.middlewares {
		decision, err := m.BeforeExecute(ctx, call)
		if err != nil {
			return schema.ToolResult{
				ToolCallID: call.ID,
				Output:     "Middleware error: " + err.Error(),
				IsError:    true,
			}
		}

		if decision.Type == middleware.DecisionDeny {
			return schema.ToolResult{
				ToolCallID: call.ID,
				Output:     "Tool execution denied by middleware: " + decision.Reason,
				IsError:    true,
			}

		}
	}

	output, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		errMsg := fmt.Sprintf("Error executing %s: %v", call.Name, err)
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     errMsg,
			IsError:    true,
		}
	}

	return schema.ToolResult{
		ToolCallID: call.ID,
		Output:     output,
		IsError:    false,
	}
}

// IsParallelSafe reports whether a tool can be executed in parallel with other tools.
// Returns true only if the tool implements ParallelSafeTool and its ParallelSafe method returns true.
// Non-existent tools are considered not parallel-safe. The name may be canonical
// or an alias.
func (r *registryImpl) IsParallelSafe(toolName string) bool {
	tool, exists := r.tools[r.resolve(toolName)]
	if !exists {
		return false
	}

	safeTool, ok := tool.(ParallelSafeTool)
	return ok && safeTool.ParallelSafe()
}
