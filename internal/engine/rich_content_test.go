package engine

import (
	"context"
	"encoding/json"
	"github.com/Zts0hg/foxharness/internal/schema"
	"testing"
)

type richResultExecutor struct{ result ToolExecutionResult }

func (r richResultExecutor) Snapshot(context.Context) (ToolSnapshot, error) {
	return targetToolSnapshot{}, nil
}
func (r richResultExecutor) Execute(context.Context, ToolSnapshot, []schema.ToolCall) (ToolBatch, error) {
	return ToolBatch{Results: []ToolExecutionResult{r.result}}, nil
}

func TestEngineCommitsOwnedRichResult(t *testing.T) {
	source := ToolExecutionResult{CallID: "image-call", ModelContent: "preview", IsError: true,
		ContentParts: []schema.ContentPart{{Type: "text", Text: "first"}, {Type: "image", ArtifactKey: "media/hash.png"}}, StructuredContent: json.RawMessage(`{"nested":[1]}`)}
	conversation := &targetTestConversation{}
	eng := NewAgentEngine(nil, richResultExecutor{result: source}, conversation, targetTestPolicy{}, nil)
	state, err := eng.executeTools(context.Background(), func(Fact) {}, targetToolSnapshot{}, 1, []schema.ToolCall{{ID: "image-call", Name: "capture"}})
	if err != nil {
		t.Fatal(err)
	}
	source.ContentParts[0].Text = "changed"
	source.StructuredContent[0] = 'X'
	msg := conversation.messages[0]
	if msg.ToolCallID != "image-call" || !msg.IsError || len(msg.ContentParts) != 2 || msg.ContentParts[0].Text != "first" || !json.Valid(msg.StructuredContent) {
		t.Fatalf("bad committed result: %#v", msg)
	}
	state.Results[0].ContentParts[0].Text = "policy mutation"
	if conversation.messages[0].ContentParts[0].Text != "first" {
		t.Fatal("policy mutates history")
	}
}
