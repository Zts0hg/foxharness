package tracing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTracerWritesSpanEventsAndAnnotations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	tracer := NewTracer(path)

	span := tracer.StartSpan("parent", "tool_call", map[string]any{"tool": "bash"})
	tracer.Annotate(span.ID(), "note", map[string]any{"message": "checking"})
	span.End("error", map[string]any{"is_error": true})

	events, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("events len = %d, want 3: %+v", len(events), events)
	}
	if events[0].Type != EventSpanStart || events[0].ParentID != "parent" || events[0].Name != "tool_call" {
		t.Fatalf("span start = %+v", events[0])
	}
	if events[1].Type != EventAnnotation || events[1].Name != "note" {
		t.Fatalf("annotation = %+v", events[1])
	}
	if events[2].Type != EventSpanEnd || events[2].Status != "error" || events[2].Attrs["is_error"] != true {
		t.Fatalf("span end = %+v", events[2])
	}
}

func TestLoadReportsParseAndOpenErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	if err := os.WriteFile(path, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "解析 trace 事件失败") {
		t.Fatalf("Load() parse error = %v", err)
	}
	if _, err := Load(filepath.Join(dir, "missing.jsonl")); err == nil || !strings.Contains(err.Error(), "打开 trace 文件失败") {
		t.Fatalf("Load() open error = %v", err)
	}
}
