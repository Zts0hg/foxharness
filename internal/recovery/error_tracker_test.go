package recovery

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestTrackerIgnoresSuccessAndRecordsFailures(t *testing.T) {
	tracker := NewTracker()
	call := schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"false"}`)}

	tracker.Record(call, schema.ToolResult{Output: "ok"})
	if tracker.ShouldInject() || tracker.BuildPrompt() != "" {
		t.Fatal("success result should not trigger recovery")
	}

	tracker.Record(call, schema.ToolResult{IsError: true, Output: "exit status 1"})
	if !tracker.ShouldInject() {
		t.Fatal("failure should trigger recovery injection")
	}
	prompt := tracker.BuildPrompt()
	for _, want := range []string{"Error Recovery Notice", "Tool: `bash`", "Failure count for same tool+arguments: 1", "exit status 1"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestTrackerCountsRepeatedFailureAndMarkInjectClearsPending(t *testing.T) {
	tracker := NewTracker()
	call := schema.ToolCall{Name: "edit_file", Arguments: []byte(`{"path":"a.go"}`)}

	tracker.Record(call, schema.ToolResult{IsError: true, Output: strings.Repeat("x", 2500)})
	tracker.Record(call, schema.ToolResult{IsError: true, Output: "still failing"})

	prompt := tracker.BuildPrompt()
	if !strings.Contains(prompt, "Failure count for same tool+arguments: 2") {
		t.Fatalf("prompt missing repeat count:\n%s", prompt)
	}
	if !strings.Contains(prompt, "禁止再次原样调用") {
		t.Fatalf("prompt missing repeated-failure directive:\n%s", prompt)
	}
	if strings.Contains(prompt, strings.Repeat("x", 2100)) {
		t.Fatal("prompt includes untruncated long output")
	}

	tracker.MarkInject()
	if tracker.ShouldInject() {
		t.Fatal("ShouldInject() = true after MarkInject, want false")
	}
}
