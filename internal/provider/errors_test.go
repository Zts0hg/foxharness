package provider

import (
	"errors"
	"testing"
)

func TestIsPromptTooLong(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"zhipu 1261", errors.New("API error code 1261: prompt exceeds max length"), true},
		{"openai context_length_exceeded", errors.New("context_length_exceeded: max 128000 tokens"), true},
		{"prompt is too long", errors.New("prompt is too long for model"), true},
		{"prompt exceeds", errors.New("Prompt exceeds maximum context length"), true},
		{"unrelated 400", errors.New("invalid_request: missing model field"), false},
		{"server error", errors.New("internal server error"), false},
		{"timeout", errors.New("request timeout"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPromptTooLong(tc.err); got != tc.want {
				t.Fatalf("IsPromptTooLong(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
