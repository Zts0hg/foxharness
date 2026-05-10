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

type EventType string

const (
	EventSpanStart  EventType = "span_start"
	EventSpanEnd    EventType = "span_end"
	EventAnnotation EventType = "annotation"
)

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

type Tracer struct {
	path    string
	traceID string
	mu      sync.Mutex
}

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

type Span struct {
	tracer   *Tracer
	traceID  string
	spanID   string
	parentID string
	name     string
	started  time.Time
}

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

func (s *Span) ID() string {
	return s.spanID
}

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
