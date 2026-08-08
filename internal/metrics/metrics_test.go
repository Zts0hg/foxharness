package metrics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestAggregatorCountsModelToolAndErrorTotals(t *testing.T) {
	agg := NewAggregator()
	agg.AddModel(10, 20, false)
	agg.AddModel(3, 4, true)
	agg.AddTool(false)
	agg.AddTool(true)

	summary := agg.Summary("sess-1")
	if summary.Type != EventRunSummary || summary.SessionID != "sess-1" {
		t.Fatalf("summary identity = %+v", summary)
	}
	if summary.TotalModelCalls != 2 || summary.TotalToolCalls != 2 {
		t.Fatalf("summary calls = %+v", summary)
	}
	if summary.TotalInputTokens != 13 || summary.TotalOutputTokens != 24 || summary.ErrorCount != 2 {
		t.Fatalf("summary counters = %+v", summary)
	}
}

func TestRecorderAppendsJSONLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.jsonl")
	recorder := NewRecorder(path)
	if err := recorder.Append(ModelCall{Type: EventModelCall, SessionID: "sess", InputTokens: 1}); err != nil {
		t.Fatalf("Append model error = %v", err)
	}
	if err := recorder.Append(ToolCall{Type: EventToolCall, SessionID: "sess", ToolName: "bash", IsError: true}); err != nil {
		t.Fatalf("Append tool error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2:\n%s", len(lines), data)
	}
	var tool map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &tool); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if tool["type"] != string(EventToolCall) || tool["is_error"] != true {
		t.Fatalf("tool line = %+v", tool)
	}
}

func TestRoughEstimatorCountsTextMessagesAndToolDefinitions(t *testing.T) {
	estimator := RoughEstimator{}
	if got := estimator.EstimateText("ab世"); got != 4 {
		t.Fatalf("EstimateText() = %d, want rune count plus one", got)
	}
	messages := []schema.Message{{
		Role:    schema.RoleAssistant,
		Content: "run",
		ToolCalls: []schema.ToolCall{{
			ID:        "call-1",
			Name:      "bash",
			Arguments: []byte(`{"command":"pwd"}`),
		}},
	}}
	if got := estimator.EstimateMessages(messages); got <= estimator.EstimateText("run") {
		t.Fatalf("EstimateMessages() = %d, want content plus tool call fields", got)
	}
	if got := EstimateToolDefinitions(estimator, []schema.ToolDefinition{{Name: "bash"}}); got == 0 {
		t.Fatal("EstimateToolDefinitions() = 0, want serialized tool estimate")
	}
}
