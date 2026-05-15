// Package tracing provides a lightweight, span-based tracing system for
// debugging agent execution. Traces are written as JSONL files, with each
// line representing a span start, span end, or annotation event. Spans
// form a tree via parent IDs and carry arbitrary key-value attributes.
package tracing

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// EventType distinguishes the kind of trace event.
type EventType string

const (
	// EventSpanStart marks the beginning of a span.
	EventSpanStart EventType = "span_start"
	// EventSpanEnd marks the completion of a span.
	EventSpanEnd EventType = "span_end"
	// EventAnnotation attaches a named key-value annotation to a span.
	EventAnnotation EventType = "annotation"
)

// SpanEvent is the unit record written to the trace file. Each event is
// one of span_start, span_end, or annotation, identified by Type.
type SpanEvent struct {
	Type       EventType      `json:"type"`
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	ParentID   string         `json:"parent_id,omitempty"`
	Name       string         `json:"name,omitempty"`
	Time       time.Time      `json:"time"`
	Status     string         `json:"status"`
	DurationMS int64          `json:"duration_ms,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
}

// Tracer writes span-based trace events to a JSONL file. All writes are
// serialized through a mutex so it is safe for concurrent use.
type Tracer struct {
	path    string
	traceID string
	mu      sync.Mutex
}

// NewTracer creates a Tracer that writes to path and assigns a random
// trace ID to all events produced by this tracer.
func NewTracer(path string) *Tracer {
	return &Tracer{
		path:    path,
		traceID: newID(),
	}
}

func newID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Span represents an in-flight span. Call End to record the span_end
// event with the elapsed duration.
type Span struct {
	tracer   *Tracer
	traceID  string
	spanID   string
	parentID string
	name     string
	started  time.Time
}

// StartSpan creates a new span under the given parent, records a
// span_start event, and returns the Span for later completion.
func (t *Tracer) StartSpan(parentID, name string, attrs map[string]any) *Span {
	span := &Span{
		tracer:   t,
		traceID:  t.traceID,
		spanID:   newID(),
		parentID: parentID,
		name:     name,
		started:  time.Now(),
	}

	_ = t.append(SpanEvent{
		Type:     EventSpanStart,
		TraceID:  span.traceID,
		SpanID:   span.spanID,
		ParentID: parentID,
		Name:     name,
		Time:     span.started,
		Attrs:    attrs,
	})

	return span
}

// ID returns the unique identifier for this span.
func (s *Span) ID() string {
	return s.spanID
}

// End records a span_end event with the given status and computes the
// elapsed duration since the span was started.
func (s *Span) End(status string, attrs map[string]any) {
	_ = s.tracer.append(SpanEvent{
		Type:       EventSpanEnd,
		TraceID:    s.traceID,
		SpanID:     s.spanID,
		Name:       s.name,
		Time:       time.Now(),
		Status:     status,
		DurationMS: time.Since(s.started).Milliseconds(),
		Attrs:      attrs,
	})
}

// Annotate records an annotation event associated with the given span.
func (t *Tracer) Annotate(spanID, name string, attrs map[string]any) {
	_ = t.append(SpanEvent{
		Type:    EventAnnotation,
		TraceID: t.traceID,
		SpanID:  spanID,
		Name:    name,
		Time:    time.Now(),
		Attrs:   attrs,
	})
}

func (t *Tracer) append(event SpanEvent) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开 trace 文件失败: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化 trace 事件失败: %w", err)
	}

	_, err = f.Write(append(data, '\n'))
	return err

}
