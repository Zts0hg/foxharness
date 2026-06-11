package autodev

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestTerminalReporterRendersOrchestrationEvents(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminalReporter(&buf)
	ctx := context.Background()

	rep.OnItemStart(ctx, 1, 2, LedgerItem{Slug: "engine-memory", Priority: PriorityHigh, Title: "Engine memory"})
	rep.OnWorktree(ctx, Worktree{Path: "../wt/engine-memory", Branch: "auto/engine-memory"})
	rep.OnStageStart(ctx, "engine-memory", "generate-spec")
	rep.OnEngineerDecision(ctx,
		[]tools.Question{{Prompt: "Where should discoveries be appended?"}},
		[]tools.Answer{{QuestionText: "Where should discoveries be appended?", Value: "MEMORY.md"}})
	rep.OnEngineerReview(ctx, "generate-spec", "spec.md is missing; write it now")
	rep.OnVerify(ctx, "generate-spec", false, "spec.md absent")
	rep.OnVerify(ctx, "generate-spec", true, "")
	rep.OnGate(ctx, GateResult{Passed: true, Steps: []GateStep{
		{Name: "build", Passed: true},
		{Name: "test", Passed: true},
		{Name: "gofmt", Passed: true},
	}})
	rep.OnIssue(ctx, 31)
	rep.OnPR(ctx, 32)
	rep.OnItemDone(ctx, LedgerItem{Slug: "engine-memory", Issue: 31, PR: 32})
	rep.OnInfo(ctx, "backlog drained")

	out := buf.String()
	for _, want := range []string{
		"engine-memory",
		"1/2",
		"auto/engine-memory",
		"generate-spec",
		"Where should discoveries be appended?",
		"MEMORY.md",
		"spec.md is missing; write it now",
		"spec.md absent",
		"build",
		"#31",
		"#32",
		"backlog drained",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q:\n%s", want, out)
		}
	}
}

func TestTerminalReporterRendersEngineEvents(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminalReporter(&buf)
	ctx := context.Background()

	rep.OnRunStart(ctx, "sess-1", "run-1")
	rep.OnToolCall(ctx, "bash", `{"command":"git add -A"}`)
	rep.OnToolResult(ctx, "bash", "ok", false)
	rep.OnMessage(ctx, "I wrote the spec.")
	rep.OnRunComplete(ctx, engine.RunResult{RunID: "run-1"})

	out := buf.String()
	for _, want := range []string{"bash", "git add -A", "I wrote the spec."} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q:\n%s", want, out)
		}
	}
}

func TestTerminalReporterImplementsReporter(t *testing.T) {
	var _ Reporter = NewTerminalReporter(&bytes.Buffer{})
}
