package reminder

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestManagerBuildsRepeatedActionReminderAndAppliesCooldown(t *testing.T) {
	manager := NewManager()
	call := schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"ls"}`)}
	for turn := 1; turn <= 3; turn++ {
		manager.Record(turn, call, schema.ToolResult{})
	}

	msg, ok := manager.MaybeBuild(4)
	if !ok || !strings.Contains(msg, "Possible Loop Detected") {
		t.Fatalf("MaybeBuild() = %q, %v; want loop reminder", msg, ok)
	}
	if msg, ok := manager.MaybeBuild(5); ok {
		t.Fatalf("MaybeBuild during cooldown = %q, true; want suppressed", msg)
	}
}

func TestManagerBuildsEditWithoutVerificationReminder(t *testing.T) {
	manager := NewManager()
	manager.Record(1, schema.ToolCall{Name: "write_file", Arguments: []byte(`{"path":"a.go"}`)}, schema.ToolResult{})
	manager.Record(2, schema.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"a.go"}`)}, schema.ToolResult{})
	manager.Record(3, schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"pwd"}`)}, schema.ToolResult{})
	manager.Record(4, schema.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"b.go"}`)}, schema.ToolResult{})
	manager.Record(5, schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"git status --short"}`)}, schema.ToolResult{})

	msg, ok := manager.MaybeBuild(6)
	if !ok || !strings.Contains(msg, "Verification Needed") {
		t.Fatalf("MaybeBuild() = %q, %v; want verification reminder", msg, ok)
	}
}

func TestManagerSuppressesEditReminderAfterVerification(t *testing.T) {
	manager := NewManager()
	manager.Record(1, schema.ToolCall{Name: "edit_file", Arguments: []byte(`{"path":"a.go"}`)}, schema.ToolResult{})
	manager.Record(2, schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"go test ./..."}`)}, schema.ToolResult{})
	manager.Record(3, schema.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"a.go"}`)}, schema.ToolResult{})
	manager.Record(4, schema.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"b.go"}`)}, schema.ToolResult{})
	manager.Record(5, schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"git status --short"}`)}, schema.ToolResult{})

	msg, ok := manager.MaybeBuild(6)
	if ok {
		t.Fatalf("MaybeBuild() = %q, true; want no reminder after verification", msg)
	}
}

func TestManagerBuildsReAnchorReminder(t *testing.T) {
	manager := NewManager()
	msg, ok := manager.MaybeBuild(12)
	if !ok || !strings.Contains(msg, "Re-anchor") {
		t.Fatalf("MaybeBuild() = %q, %v; want re-anchor reminder", msg, ok)
	}
}

func TestPolicyReminderPriorityCooldownReanchorAndVerificationSuppression(t *testing.T) {
	priority := NewManager()
	repeated := schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"ls"}`)}
	for turn := 1; turn <= 3; turn++ {
		priority.Record(turn, repeated, schema.ToolResult{})
	}
	message, ok := priority.MaybeBuild(12)
	if !ok || !strings.Contains(message, "Possible Loop Detected") || strings.Contains(message, "Re-anchor") {
		t.Fatalf("priority reminder = %q, %v; want loop before re-anchor", message, ok)
	}
	if message, ok := priority.MaybeBuild(13); ok {
		t.Fatalf("cooldown reminder = %q, true; want suppression", message)
	}

	verified := NewManager()
	verified.Record(1, schema.ToolCall{Name: "edit_file", Arguments: []byte(`{"path":"a.go"}`)}, schema.ToolResult{})
	verified.Record(2, schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"go test ./..."}`)}, schema.ToolResult{})
	verified.Record(3, schema.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"a.go"}`)}, schema.ToolResult{})
	verified.Record(4, schema.ToolCall{Name: "read_file", Arguments: []byte(`{"path":"b.go"}`)}, schema.ToolResult{})
	verified.Record(5, schema.ToolCall{Name: "bash", Arguments: []byte(`{"command":"git status --short"}`)}, schema.ToolResult{})
	message, ok = verified.MaybeBuild(12)
	if !ok || !strings.Contains(message, "Re-anchor") || strings.Contains(message, "Verification Needed") {
		t.Fatalf("verified reminder = %q, %v; want verification suppression then re-anchor", message, ok)
	}
}
