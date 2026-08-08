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
