package provider

import (
	"net/http"
	"strings"
)

// IsPromptTooLong reports whether err indicates that the API rejected the
// request because the prompt exceeds the model's maximum input length.
// It checks for Zhipu error code 1261, OpenAI's context_length_exceeded,
// and common prompt-too-long message patterns.
func IsPromptTooLong(err error) bool {
	if err == nil {
		return false
	}

	if code, ok := errorStatusCode(err); ok && code != http.StatusBadRequest {
		return false
	}

	msg := strings.ToLower(err.Error())
	for _, pattern := range []string{
		"1261",
		"context_length_exceeded",
		"prompt is too long",
		"prompt exceeds",
		"maximum context length",
		"max prompt length",
	} {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}
