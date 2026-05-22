package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type recordingCheckpointer struct {
	calls []trackCall
}

type trackCall struct {
	path      string
	messageID string
}

func (c *recordingCheckpointer) TrackEdit(filePath, messageID string) error {
	c.calls = append(c.calls, trackCall{path: filePath, messageID: messageID})
	return nil
}
func (c *recordingCheckpointer) MakeSnapshot(messageID string) error { return nil }
func (c *recordingCheckpointer) Rewind(messageID string) ([]string, error) {
	return nil, nil
}
func (c *recordingCheckpointer) GetDiffStats(messageID string) (*checkpoint.DiffStats, error) {
	return nil, nil
}
func (c *recordingCheckpointer) HasAnyChanges(messageID string) (bool, error) { return false, nil }
func (c *recordingCheckpointer) SetDisabled(disabled bool)                    {}
func (c *recordingCheckpointer) IsDisabled() bool                             { return false }
func (c *recordingCheckpointer) RestoreStateFromLog() error                   { return nil }

func TestCheckpointMiddleware(t *testing.T) {
	cp := &recordingCheckpointer{}
	m := NewCheckpointMiddleware(cp, func() string { return "42" })

	for _, name := range []string{"write_file", "edit_file", "bash"} {
		args, err := json.Marshal(map[string]string{"path": "main.go"})
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}
		decision, err := m.BeforeExecute(context.Background(), schema.ToolCall{
			ID:        "call-" + name,
			Name:      name,
			Arguments: args,
		})
		if err != nil {
			t.Fatalf("BeforeExecute(%s) error = %v", name, err)
		}
		if decision.Type != DecisionAllow {
			t.Fatalf("BeforeExecute(%s) decision = %#v, want allow", name, decision)
		}
	}

	if len(cp.calls) != 2 {
		t.Fatalf("TrackEdit calls = %#v, want 2 calls", cp.calls)
	}
	for _, call := range cp.calls {
		if call.path != "main.go" || call.messageID != "42" {
			t.Fatalf("TrackEdit call = %#v, want main.go/42", call)
		}
	}
}
