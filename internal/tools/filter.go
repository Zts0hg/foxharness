package tools

import (
	"context"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
)

// NewFilteredRegistry wraps base with an allow-list of tool names. When
// allowed is nil, the base registry is returned unchanged. The wrapper is
// only created when a caller explicitly restricts the tool surface for a
// single run — for example a slash command with `allowed-tools` (TUI
// path) or a sub-agent invoked with a restricted skill (fork path).
//
// The wrapper satisfies tools.Registry and is intended to be a per-run
// view layered on top of a shared base registry; it intentionally does
// not allow new tool registration (Register is a no-op) so a filtered
// view cannot leak tools into the underlying registry.
func NewFilteredRegistry(base Registry, allowed []string) Registry {
	if allowed == nil {
		return base
	}
	set := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		set[name] = true
	}
	return &filteredRegistry{base: base, allowed: set}
}

type filteredRegistry struct {
	base    Registry
	allowed map[string]bool
}

func (f *filteredRegistry) Register(tool BaseTool) {
	// Filtering does not permit registration; the underlying registry is
	// shared and we should not let one filtered view leak tools into it.
}

func (f *filteredRegistry) Use(m middleware.Middleware) {
	f.base.Use(m)
}

func (f *filteredRegistry) GetAvailableTools() []schema.ToolDefinition {
	all := f.base.GetAvailableTools()
	out := make([]schema.ToolDefinition, 0, len(all))
	for _, def := range all {
		if f.allowed[def.Name] {
			out = append(out, def)
		}
	}
	return out
}

func (f *filteredRegistry) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	if !f.allowed[call.Name] {
		return schema.ToolResult{
			ToolCallID: call.ID,
			Output:     fmt.Sprintf("Tool %q not permitted by the active allow-list", call.Name),
			IsError:    true,
		}
	}
	return f.base.Execute(ctx, call)
}

func (f *filteredRegistry) IsParallelSafe(toolName string) bool {
	if !f.allowed[toolName] {
		return false
	}
	return f.base.IsParallelSafe(toolName)
}

// BeginTurn forwards the optional turn boundary to the wrapped registry.
func (f *filteredRegistry) BeginTurn() {
	if turnAware, ok := f.base.(TurnAwareRegistry); ok {
		turnAware.BeginTurn()
	}
}
