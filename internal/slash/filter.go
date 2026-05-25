package slash

import (
	"context"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// NewFilteredRegistry wraps base with an allow-list of tool names. When
// allowed is nil, the base registry is returned unchanged — the wrapper is
// only created when the command frontmatter explicitly restricts tools.
//
// The wrapper satisfies tools.Registry and is intended to be supplied to
// the engine for the duration of a single command's execution; it does not
// allow new tools to be registered (Register is a no-op).
func NewFilteredRegistry(base tools.Registry, allowed []string) tools.Registry {
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
	base    tools.Registry
	allowed map[string]bool
}

func (f *filteredRegistry) Register(tool tools.BaseTool) {
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
			Output:     fmt.Sprintf("Tool %q not permitted by command's allowed-tools restriction", call.Name),
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
