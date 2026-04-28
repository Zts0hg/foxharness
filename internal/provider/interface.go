package provider

import (
	"context"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type LLMProvider interface {
	Generate(ctx context.Context, message []schema.Message, availableTools []schema.ToolDefinition) (*schema.Message, error)
}
