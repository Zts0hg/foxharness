package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestImageRejectionRequiresExplicitBadRequest(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{structuredOutputStatusError{StatusCode: 400, message: "model does not support image input"}, true},
		{structuredOutputStatusError{StatusCode: 401, message: "model does not support image input"}, false},
		{structuredOutputStatusError{StatusCode: 500, message: "model does not support image input"}, false},
		{structuredOutputStatusError{StatusCode: 400, message: "invalid image encoding"}, false},
		{errors.New("model does not support image input"), false},
	} {
		if got := IsImageInputUnsupported(tc.err); got != tc.want {
			t.Fatalf("classification %v: %v", tc.err, got)
		}
	}
}

func TestProviderRequestsContainCorrelatedImageBytes(t *testing.T) {
	for _, protocol := range []string{"openai", "claude"} {
		t.Run(protocol, func(t *testing.T) {
			var captured map[string]any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(body, &captured); err != nil {
					t.Error(err)
				}
				w.Header().Set("Content-Type", "application/json")
				if protocol == "openai" {
					io.WriteString(w, `{"id":"r","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"seen"},"finish_reason":"stop"}]}`)
				} else {
					io.WriteString(w, `{"id":"r","type":"message","role":"assistant","model":"image-model","content":[{"type":"text","text":"seen"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
				}
			}))
			defer server.Close()
			p, err := NewProvider(llmconfig.ResolvedConfig{Protocol: protocol, BaseURL: server.URL, Model: "image-model", Auth: "none"})
			if err != nil {
				t.Fatal(err)
			}
			data := []byte("actual image bytes")
			messages := []schema.Message{
				{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{{ID: "shot", Name: "capture", Arguments: json.RawMessage(`{}`)}, {ID: "other", Name: "read", Arguments: json.RawMessage(`{}`)}}},
				{Role: schema.RoleUser, ToolCallID: "shot", Content: "page", IsError: true, ContentParts: []schema.ContentPart{{Type: "image", MIMEType: "image/png", ArtifactKey: "media/example.png", Data: data}}},
				{Role: schema.RoleUser, ToolCallID: "other", Content: "other result"},
			}
			if _, err := p.Generate(context.Background(), messages, nil); err != nil {
				t.Fatal(err)
			}
			got := captured["messages"].([]any)
			if protocol == "openai" {
				if len(got) != 4 || got[1].(map[string]any)["tool_call_id"] != "shot" || got[2].(map[string]any)["tool_call_id"] != "other" || got[3].(map[string]any)["role"] != "user" {
					t.Fatalf("batch ordering: %#v", got)
				}
				parts := got[3].(map[string]any)["content"].([]any)
				encoded, _ := json.Marshal(parts)
				if !strings.Contains(string(encoded), "shot") || !strings.Contains(string(encoded), "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data)) {
					t.Fatalf("missing actual image: %s", encoded)
				}
			} else {
				block := got[1].(map[string]any)["content"].([]any)[0].(map[string]any)
				if block["tool_use_id"] != "shot" || block["is_error"] != true {
					t.Fatalf("correlation/error: %#v", block)
				}
				parts := block["content"].([]any)
				found := false
				for _, raw := range parts {
					p := raw.(map[string]any)
					if p["type"] == "image" {
						source := p["source"].(map[string]any)
						found = source["type"] == "base64" && source["media_type"] == "image/png" && source["data"] == base64.StdEncoding.EncodeToString(data)
					}
				}
				if !found {
					t.Fatalf("no image data: %#v", parts)
				}
			}
		})
	}
}
