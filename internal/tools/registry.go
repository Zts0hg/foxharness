package tools

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type Registry interface {
	GetAvailableTools() []schema.ToolDefinition
	Execute(ctx context.Context, call schema.ToolCall) schema.ToolReuslt
}
