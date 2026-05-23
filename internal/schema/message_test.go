package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestUsageJSONRoundTrip(t *testing.T) {
	t.Run("zero value omits optional cache fields", func(t *testing.T) {
		u := Usage{InputTokens: 100, OutputTokens: 50}
		data, err := json.Marshal(u)
		if err != nil {
			t.Fatalf("Marshal(Usage) error = %v", err)
		}
		got := string(data)
		if !strings.Contains(got, `"input_tokens":100`) {
			t.Fatalf("Marshal(Usage) = %q, want to contain input_tokens", got)
		}
		if !strings.Contains(got, `"output_tokens":50`) {
			t.Fatalf("Marshal(Usage) = %q, want to contain output_tokens", got)
		}
		if strings.Contains(got, "cache_creation_tokens") {
			t.Fatalf("Marshal(Usage) = %q, want cache_creation_tokens omitted", got)
		}
		if strings.Contains(got, "cache_read_tokens") {
			t.Fatalf("Marshal(Usage) = %q, want cache_read_tokens omitted", got)
		}
	})

	t.Run("all fields round-trip", func(t *testing.T) {
		want := Usage{
			InputTokens:         1000,
			OutputTokens:        500,
			CacheCreationTokens: 250,
			CacheReadTokens:     750,
		}
		data, err := json.Marshal(want)
		if err != nil {
			t.Fatalf("Marshal(Usage) error = %v", err)
		}
		var got Usage
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal(Usage) error = %v", err)
		}
		if got != want {
			t.Fatalf("Usage round-trip = %#v, want %#v", got, want)
		}
	})
}

func TestMessageWithUsage(t *testing.T) {
	want := Message{
		Role:    RoleAssistant,
		Content: "hello",
		Usage: &Usage{
			InputTokens:  100,
			OutputTokens: 20,
		},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}
	var got Message
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error = %v", err)
	}
	if got.Usage == nil {
		t.Fatalf("Usage should not be nil after round-trip")
	}
	if *got.Usage != *want.Usage {
		t.Fatalf("Usage = %#v, want %#v", got.Usage, want.Usage)
	}
}

func TestMessageWithoutUsage(t *testing.T) {
	t.Run("nil usage omitted from JSON", func(t *testing.T) {
		msg := Message{Role: RoleUser, Content: "hi"}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal error = %v", err)
		}
		if strings.Contains(string(data), "usage") {
			t.Fatalf("Marshal(Message) = %q, want no usage field", string(data))
		}
	})

	t.Run("legacy JSON without usage loads as nil", func(t *testing.T) {
		raw := []byte(`{"role":"user","content":"legacy"}`)
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("Unmarshal(legacy) error = %v", err)
		}
		if msg.Usage != nil {
			t.Fatalf("legacy Message.Usage = %#v, want nil", msg.Usage)
		}
		if msg.Content != "legacy" {
			t.Fatalf("legacy Message.Content = %q, want legacy", msg.Content)
		}
	})
}

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
