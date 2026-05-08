package metrics

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

type EventType string

const (
	EventModelCall  EventType = "model_call"
	EventToolCall   EventType = "tool_call"
	EventRunSummary EventType = "run_summary"
)

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

type Recorder struct {
	path string
	mu   sync.Mutex
}

func NewRecorder(path string) *Recorder {
	return &Recorder{path: path}
}

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
