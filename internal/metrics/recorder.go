// Package metrics provides session-level recording and aggregation of
// agent runtime metrics including model call token usage, tool call
// performance, and per-run summaries. Events are appended as JSONL for
// later analysis.
package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// EventType distinguishes the kind of metrics event being recorded.
type EventType string

const (
	// EventModelCall records a single LLM model invocation.
	EventModelCall EventType = "model_call"
	// EventToolCall records a single tool invocation.
	EventToolCall EventType = "tool_call"
	// EventRunSummary records an aggregated summary at the end of a run.
	EventRunSummary EventType = "run_summary"
)

// ModelCall captures token usage and latency for one LLM invocation.
type ModelCall struct {
	Time         time.Time `json:"time"`
	Type         EventType `json:"type"`
	SessionID    string    `json:"session_id"`
	Turn         int       `json:"turn"`
	Phase        string    `json:"phase"`
	Model        string    `json:"model,omitempty"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	DurationMS   int64     `json:"duration_ms"`
	Error        string    `json:"error"`
}

// ToolCall captures timing and result metadata for one tool invocation.
type ToolCall struct {
	Time        time.Time `json:"time"`
	Type        EventType `json:"type"`
	SessionID   string    `json:"sesssion_id"`
	Turn        int       `json:"turn"`
	ToolName    string    `json:"tool_name"`
	ToolCallID  string    `json:"tool_call_id"`
	DurationMS  int64     `json:"duration_ms"`
	OutputBytes int       `json:"output_bytes"`
	IsError     bool      `json:"is_error"`
}

// RunSummary aggregates all metrics for a completed agent run.
type RunSummary struct {
	Time              time.Time `json:"time"`
	Type              EventType `json:"type"`
	SessionID         string    `json:"session_id"`
	TotalModelCalls   int       `json:"total_model_calls"`
	TotalToolCalls    int       `json:"total_tool_calls"`
	TotalInputTokens  int       `json:"total_input_tokens"`
	TotalOutputTokens int       `json:"total_output_tokens"`
	TotalDurationMS   int64     `json:"total_duration_ms"`
	ErrorCount        int       `json:"error_count"`
}

// Recorder appends JSONL metric events to a single file in a
// concurrency-safe manner.
type Recorder struct {
	path string
	mu   sync.Mutex
}

// NewRecorder creates a Recorder that writes events to path.
func NewRecorder(path string) *Recorder {
	return &Recorder{path: path}
}

// Append serializes event as JSON and appends it as a new line to the
// underlying file. The file is created if it does not exist.
func (r *Recorder) Append(event any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	f, err := os.OpenFile(r.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开 metrics 文件失败: %w", err)
	}

	defer f.Close()

	data, err := json.Marshal(event)

	if err != nil {
		return fmt.Errorf("序列化 metrics 事件失败: %w", err)
	}

	_, err = f.Write(append(data, '\n'))
	return err
}
