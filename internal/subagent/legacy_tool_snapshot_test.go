package subagent

import (
	"context"
	"encoding/json"

	"github.com/Zts0hg/foxharness/internal/middleware"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type childToolSnapshot struct {
	registry    tools.Registry
	definitions []schema.ToolDefinition
}

func newChildToolSnapshot(registry tools.Registry) *childToolSnapshot {
	return &childToolSnapshot{
		registry:    registry,
		definitions: cloneToolDefinitions(registry.GetAvailableTools()),
	}
}

func (s *childToolSnapshot) Register(tools.BaseTool) {}

func (s *childToolSnapshot) Use(middleware middleware.Middleware) {
	s.registry.Use(middleware)
}

func (s *childToolSnapshot) GetAvailableTools() []schema.ToolDefinition {
	return cloneToolDefinitions(s.definitions)
}

func (s *childToolSnapshot) Execute(ctx context.Context, call schema.ToolCall) schema.ToolResult {
	return s.registry.Execute(ctx, call)
}

func (s *childToolSnapshot) IsParallelSafe(name string) bool {
	return s.registry.IsParallelSafe(name)
}

func (s *childToolSnapshot) AssessPermission(name string, ctx toolpolicy.Context, args json.RawMessage) (toolpolicy.Assessment, bool, error) {
	registry, ok := s.registry.(tools.PermissionRegistry)
	if !ok {
		return toolpolicy.Assessment{}, false, nil
	}
	return registry.AssessPermission(name, ctx, args)
}

func (s *childToolSnapshot) BeginTurn() {
	if registry, ok := s.registry.(tools.TurnAwareRegistry); ok {
		registry.BeginTurn()
	}
}

func (s *childToolSnapshot) capabilityNames() []string {
	names := make([]string, 0, len(s.definitions))
	for _, definition := range s.definitions {
		names = append(names, definition.Name)
	}
	return names
}

func cloneToolDefinitions(definitions []schema.ToolDefinition) []schema.ToolDefinition {
	cloned := make([]schema.ToolDefinition, len(definitions))
	for i, definition := range definitions {
		cloned[i] = definition
		cloned[i].InputSchema = cloneSchemaValue(definition.InputSchema)
	}
	return cloned
}

func cloneSchemaValue(value interface{}) interface{} {
	switch value := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(value))
		for key, nested := range value {
			cloned[key] = cloneSchemaValue(nested)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(value))
		for i, nested := range value {
			cloned[i] = cloneSchemaValue(nested)
		}
		return cloned
	case []string:
		return append([]string(nil), value...)
	case map[string]string:
		cloned := make(map[string]string, len(value))
		for key, nested := range value {
			cloned[key] = nested
		}
		return cloned
	case json.RawMessage:
		return append(json.RawMessage(nil), value...)
	default:
		return value
	}
}
