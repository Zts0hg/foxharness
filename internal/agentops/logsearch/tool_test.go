package logsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
)

type failOnSecondReadCloser struct {
	reads int
}

func (r *failOnSecondReadCloser) Read(p []byte) (int, error) {
	r.reads++
	if r.reads > 1 {
		return 0, fmt.Errorf("reader drained past search limit")
	}
	return copy(p, "ERROR first match\n"), nil
}

func (r *failOnSecondReadCloser) Close() error {
	return nil
}

func TestStopsAfterLimitWithoutDrainingReader(t *testing.T) {
	reader := &failOnSecondReadCloser{}
	tool := New(t.TempDir())
	tool.openLog = func(string) (io.ReadCloser, error) {
		return reader, nil
	}
	raw, err := json.Marshal(map[string]any{"service": "payment", "query": "error", "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	got, err := tool.Execute(context.Background(), raw)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(got, "ERROR first match") || reader.reads != 1 {
		t.Fatalf("Execute() = %q with %d reads, want first match with one read", got, reader.reads)
	}
}

func TestHonorsCanceledContextBeforeScanning(t *testing.T) {
	tool := New(t.TempDir())
	tool.openLog = func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("ERROR should not scan\n")), nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	raw, err := json.Marshal(map[string]any{"service": "payment", "query": "error", "limit": 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tool.Execute(ctx, raw); err == nil {
		t.Fatal("Execute() error = nil, want context cancellation")
	}
}
