package toolruntime

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolexec"
	"github.com/Zts0hg/foxharness/internal/toolresult"
)

func TestExecutorPreservesSmallStructuredResults(t *testing.T) {
	executor := New([]toolexec.Capability{{
		Definition: schema.ToolDefinition{Name: "inspect"},
		Execute: func(context.Context, schema.ToolCall) engine.ToolExecutionResult {
			return engine.ToolExecutionResult{FullContent: "full", ModelContent: "model", ObserverContent: "observer"}
		},
	}}, toolresult.OSFileSystem{}, t.TempDir())
	snapshot, err := executor.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	batch, err := executor.Execute(context.Background(), snapshot, []schema.ToolCall{{ID: "call-1", Name: "inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	result := batch.Results[0]
	if result.CallID != "call-1" || result.FullContent != "full" || result.ModelContent != "model" || result.ObserverContent != "observer" || result.ArtifactPath != "" {
		t.Fatalf("small result = %#v", result)
	}
}

func TestExecutorCapsPersistsAndSeparatesLargeResultForms(t *testing.T) {
	original := strings.Repeat("x", toolresult.MaxToolResultBytes+10_000)
	dir := t.TempDir()
	executor := New([]toolexec.Capability{{
		Definition: schema.ToolDefinition{Name: "inspect"},
		Execute: func(context.Context, schema.ToolCall) engine.ToolExecutionResult {
			return engine.ToolExecutionResult{FullContent: original, ModelContent: original, ObserverContent: original}
		},
	}}, toolresult.OSFileSystem{}, dir)
	snapshot, _ := executor.Snapshot(context.Background())
	batch, err := executor.Execute(context.Background(), snapshot, []schema.ToolCall{{ID: "call-large", Name: "inspect"}})
	if err != nil {
		t.Fatal(err)
	}
	result := batch.Results[0]
	if len(result.FullContent) >= len(original) || !strings.Contains(result.FullContent, "truncated at 400KB") {
		t.Fatalf("full capped result length = %d", len(result.FullContent))
	}
	if result.ArtifactPath == "" || !strings.Contains(result.ModelContent, "<persisted-output>") || !strings.Contains(result.ModelContent, result.ArtifactPath) {
		t.Fatalf("persisted result = %#v", result)
	}
	if len(result.ObserverContent) > 900 || !strings.Contains(result.ObserverContent, "已截断") {
		t.Fatalf("observer result length/content = %d/%q", len(result.ObserverContent), result.ObserverContent)
	}
}

func TestExecutorRejectsTypedNilSnapshot(t *testing.T) {
	executor := New(nil, toolresult.OSFileSystem{}, t.TempDir())
	var typedNil *snapshot
	var frozen engine.ToolSnapshot = typedNil
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Execute() panicked for typed-nil snapshot: %v", recovered)
		}
	}()
	if _, err := executor.Execute(context.Background(), frozen, nil); err == nil {
		t.Fatal("Execute() error = nil, want ownership error for typed-nil snapshot")
	}
}

func TestDynamicExecutorRefreshesCapabilitiesAfterTurnBoundaryAndFreezesEachSnapshot(t *testing.T) {
	phase := 0
	executor := NewDynamic(func() []toolexec.Capability {
		name := "first"
		if phase > 0 {
			name = "second"
		}
		return []toolexec.Capability{{Definition: schema.ToolDefinition{Name: name}}}
	}, func(context.Context) error {
		phase++
		return nil
	}, toolresult.OSFileSystem{}, t.TempDir())

	first, err := executor.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := first.ToolDefinitions()[0].Name; got != "first" {
		t.Fatalf("first snapshot = %q", got)
	}
	if err := executor.BeginTurn(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, err := executor.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := second.ToolDefinitions()[0].Name; got != "second" {
		t.Fatalf("second snapshot = %q", got)
	}
	if got := first.ToolDefinitions()[0].Name; got != "first" {
		t.Fatalf("first snapshot mutated to %q", got)
	}
}
