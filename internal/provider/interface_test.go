package provider

import (
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestGenerateResponseAccess(t *testing.T) {
	msg := &schema.Message{Role: schema.RoleAssistant, Content: "hi"}
	resp := &GenerateResponse{
		Message: msg,
		Usage: schema.Usage{
			InputTokens:  100,
			OutputTokens: 25,
		},
	}

	if resp.Message != msg {
		t.Fatalf("Message identity should be preserved through GenerateResponse")
	}
	if resp.Message.Content != "hi" {
		t.Fatalf("Message.Content = %q, want hi", resp.Message.Content)
	}
	if resp.Usage.InputTokens != 100 {
		t.Fatalf("Usage.InputTokens = %d, want 100", resp.Usage.InputTokens)
	}
	if resp.Usage.OutputTokens != 25 {
		t.Fatalf("Usage.OutputTokens = %d, want 25", resp.Usage.OutputTokens)
	}
}
