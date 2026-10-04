package registryexec

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolresult"
	"github.com/Zts0hg/foxharness/internal/toolruntime"
	"github.com/Zts0hg/foxharness/internal/tools"
)

type richTool struct{ result tools.ExecutionResult }

func (r richTool) Name() string                      { return "rich" }
func (r richTool) Definition() schema.ToolDefinition { return schema.ToolDefinition{Name: "rich"} }
func (r richTool) Execute(context.Context, json.RawMessage) (string, error) {
	panic("ResultTool required")
}
func (r richTool) ExecuteResult(context.Context, json.RawMessage) (tools.ExecutionResult, error) {
	return r.result, nil
}

func TestRichRegistryExecutionPreservesContentAcrossTruncationAndHookMutation(t *testing.T) {
	r := tools.NewRegistry()
	source := tools.ExecutionResult{Output: strings.Repeat("long", 20000), Failed: true,
		ContentParts:      []schema.ContentPart{{Type: "text", Text: "first"}, {Type: "image", ArtifactKey: "media/hash.png"}},
		StructuredContent: json.RawMessage(`{"nested":{"ok":true}}`)}
	r.Register(richTool{result: source})
	capabilities := Capabilities(r, []string{"rich"}, func(_ schema.ToolCall, result schema.ToolResult) {
		result.ContentParts[0].Text = "hook mutation"
		result.StructuredContent[0] = 'X'
	})
	exec := toolruntime.New(capabilities, toolresult.OSFileSystem{}, t.TempDir())
	snapshot, err := exec.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	batch, err := exec.Execute(context.Background(), snapshot, []schema.ToolCall{{ID: "one", Name: "rich"}, {ID: "two", Name: "rich"}})
	if err != nil {
		t.Fatal(err)
	}
	source.ContentParts[0].Text = "caller mutation"
	source.StructuredContent[0] = 'Y'
	for i, result := range batch.Results {
		if result.CallID != []string{"one", "two"}[i] || !result.IsError || len(result.ModelContent) >= len(source.Output) || result.ArtifactPath == "" || len(result.ContentParts) != 2 || result.ContentParts[0].Text != "first" || !json.Valid(result.StructuredContent) {
			t.Fatalf("rich result lost/mutated: %#v", result)
		}
	}
}
