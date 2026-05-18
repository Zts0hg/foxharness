package schema

import (
	"encoding/json"
	"testing"
)

func TestNormalizeToolArgumentsReplacesEmptyRawMessage(t *testing.T) {
	got := NormalizeToolArguments(json.RawMessage{})
	if string(got) != "{}" {
		t.Fatalf("NormalizeToolArguments(empty) = %q, want {}", string(got))
	}
	if !json.Valid(got) {
		t.Fatalf("normalized arguments are not valid JSON: %q", string(got))
	}
}

func TestNormalizeMessageMakesToolCallsMarshalable(t *testing.T) {
	msg := NormalizeMessage(Message{
		Role: RoleAssistant,
		ToolCalls: []ToolCall{{
			ID:        "call-1",
			Name:      "read_file",
			Arguments: json.RawMessage{},
		}},
	})

	if string(msg.ToolCalls[0].Arguments) != "{}" {
		t.Fatalf("arguments = %q, want {}", string(msg.ToolCalls[0].Arguments))
	}
	if _, err := json.Marshal(msg); err != nil {
		t.Fatalf("Marshal(normalized message) error = %v", err)
	}
}

func TestNormalizeToolArgumentsKeepsValidJSON(t *testing.T) {
	got := NormalizeToolArguments(json.RawMessage(` {"path":"README.md"} `))
	if string(got) != `{"path":"README.md"}` {
		t.Fatalf("NormalizeToolArguments(valid) = %q", string(got))
	}
}
