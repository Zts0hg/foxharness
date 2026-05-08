package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type Registry interface {
	Register(tool BaseTool)
	Use(middleware middleware.Middleware)
	GetAvailableTools() []schema.ToolDefinition
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult
	IsParallelSafe(toolName string) bool
}

type BaseTool interface {
	Name() string
	Definition() schema.ToolDefinition
	Execute(ctx context.Context, args json.RawMessage) (string, error)
}

type ParallelSafeTool interface {
	ParallelSafe() bool
}

type registryImpl struct {
	tools       map[string]BaseTool
	middlewares []middleware.Middleware
}

func (r *registryImpl) Use(m middleware.Middleware) {
	r.middlewares = append(r.middlewares, m)
}

func NewRegistry() Registry {
	return &registryImpl{
		tools: make(map[string]BaseTool),
	}
}

func (r *registryImpl) Register(tool BaseTool) {
	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		log.Printf("[Warning] 工具 '%s' 已经被注册，将被覆盖。\n", name)
	}

	r.tools[name] = tool
	log.Printf("[Registry] 成功挂载工具: %s\n", name)
}

func (r *registryImpl) GetAvailableTools() []schema.ToolDefinition {
	var defs []schema.ToolDefinition
	for _, tool := range r.tools {
		defs = append(defs, tool.Definition())
	}
	return defs
}

func (r *registryImpl) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	tool, exists := r.tools[call.Name]
	if !exists {
		errMsg := fmt.Sprintf("Error: 系统中不存在 '%s' 的工具。", call.Name)
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

func (r *registryImpl) IsParallelSafe(toolName string) bool {
	tool, exists := r.tools[toolName]
	if !exists {
		return false
	}

	safeTool, ok := tool.(ParallelSafeTool)
	return ok && safeTool.ParallelSafe()
}
