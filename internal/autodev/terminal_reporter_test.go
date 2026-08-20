package autodev

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

type failingReporterWriter struct{}

func (failingReporterWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestTerminalReporterRendersOrchestrationEvents(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminalReporter(&buf)
	ctx := context.Background()

	rep.OnItemStart(ctx, 1, 2, LedgerItem{Slug: "engine-memory", Priority: PriorityHigh, Title: "Engine memory"})
	rep.OnWorktree(ctx, Worktree{Path: "../wt/engine-memory", Branch: "auto/engine-memory"})
	rep.OnStageStart(ctx, "engine-memory", "generate-spec")
	rep.OnEngineerDecision(ctx,
		[]Question{{Prompt: "Where should discoveries be appended?"}},
		[]Answer{{QuestionText: "Where should discoveries be appended?", Value: "MEMORY.md"}})
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
	rep.OnRunComplete(ctx, CoreRunResult{RunID: "run-1"})

	out := buf.String()
	for _, want := range []string{"bash", "git add -A", "I wrote the spec."} {
		if !strings.Contains(out, want) {
			t.Errorf("terminal output missing %q:\n%s", want, out)
		}
	}
}

func TestUIAUT002TerminalReporterMapsCompleteOrderedEventStream(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminalReporter(&buf)
	ctx := context.Background()

	rep.OnRunStart(ctx, "session-1", "run-1")
	rep.OnThinking(ctx, 2)
	rep.OnCompaction(ctx, "automatic")
	rep.OnToolCall(ctx, "bash", `{"command":"git status"}`)
	rep.OnToolResult(ctx, "bash", "failed", true)
	rep.OnToolResult(ctx, "bash", "ok", false)
	rep.OnMessage(ctx, "first line\nsecond line")
	rep.OnRunComplete(ctx, CoreRunResult{RunID: "run-1"})
	rep.OnRunError(ctx, "session-1", "run-2", errors.New("model failed"))
	rep.OnItemStart(ctx, 1, 2, LedgerItem{Slug: "item", Priority: PriorityHigh})
	rep.OnWorktree(ctx, Worktree{Path: "/tmp/item", Branch: "auto/item"})
	rep.OnStageStart(ctx, "item", "generate-spec")
	rep.OnEngineerDecision(ctx, []Question{{Prompt: "Proceed?"}}, []Answer{{Value: "Yes"}})
	rep.OnEngineerReview(ctx, "generate-spec", "write spec\nthen review")
	rep.OnVerify(ctx, "generate-spec", true, "")
	rep.OnVerify(ctx, "generate-spec", false, "spec missing")
	rep.OnGate(ctx, GateResult{Steps: []GateStep{
		{Name: "build", Passed: true},
		{Name: "test"},
		{Name: "gofmt", Skipped: true},
	}})
	rep.OnIssue(ctx, 30)
	if err := rep.OnRemoteEvent(ctx, RemoteEvent{EventID: "issue:item:31", Kind: RemoteEventIssue, Number: 31}); err != nil {
		t.Fatal(err)
	}
	rep.OnPR(ctx, 32)
	rep.OnItemDone(ctx, LedgerItem{Slug: "item", Issue: 31, PR: 32})
	rep.OnInfo(ctx, "WARNING: failed to remove worktree: busy")

	want := "" +
		"  core   → run run-1 started\n" +
		"  core   → thinking (turn 2)\n" +
		"  core   → context compacted (automatic)\n" +
		"  core   → tool bash {\"command\":\"git status\"}\n" +
		"  core   ← tool bash ✗ failed\n" +
		"  core   ← tool bash ✓ ok\n" +
		"  core   → first line\n" +
		"           second line\n" +
		"  core   → run run-1 complete\n" +
		"  core   ✗ run run-2 error: model failed\n" +
		"[autodev] item 1/2  high  item\n" +
		"[autodev] worktree /tmp/item  branch auto/item\n" +
		"[stage] generate-spec\n" +
		"  core   → asks: Proceed?\n" +
		"  engineer → Yes\n" +
		"  engineer → write spec\n" +
		"           then review\n" +
		"[stage] generate-spec  DONE\n" +
		"[stage] generate-spec  NOT DONE: spec missing\n" +
		"[gate] build ✓  test ✗  gofmt skipped\n" +
		"[remote] issue #30\n" +
		"[remote] issue #31\n" +
		"[remote] PR #32\n" +
		"[ledger] item = done (issue #31, pr #32)\n" +
		"[autodev] WARNING: failed to remove worktree: busy\n"
	if got := buf.String(); got != want {
		t.Fatalf("terminal event stream:\n%s\nwant:\n%s", got, want)
	}
}

func TestTerminalReporterImplementsReporter(t *testing.T) {
	var _ Reporter = NewTerminalReporter(&bytes.Buffer{})
}

func TestTerminalReporterRemoteEventWriteFailureIsRetryable(t *testing.T) {
	rep := NewTerminalReporter(failingReporterWriter{})
	event := RemoteEvent{EventID: "issue:item-terminal:31", ItemID: "item-terminal", Kind: RemoteEventIssue, Number: 31}
	if err := rep.OnRemoteEvent(context.Background(), event); err == nil {
		t.Fatal("OnRemoteEvent returned nil for failed terminal write")
	}
	if _, delivered := rep.delivered[event.EventID]; delivered {
		t.Fatal("failed terminal write was marked delivered")
	}
}

func TestUIAUT005TerminalReporterFormattingAndConcurrentSerialization(t *testing.T) {
	if got, want := oneLine("  alpha\n beta\t界界界界界界界界界界  ", 15), "alpha beta 界..."; got != want {
		t.Fatalf("oneLine() = %q, want %q", got, want)
	}
	if got, want := indentContinuations(" first\nsecond\nthird "), "first\n           second\n           third"; got != want {
		t.Fatalf("indentContinuations() = %q, want %q", got, want)
	}

	const count = 64
	var buf bytes.Buffer
	rep := NewTerminalReporter(&buf)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rep.OnInfo(context.Background(), fmt.Sprintf("event-%d", i))
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != count {
		t.Fatalf("serialized line count = %d, want %d:\n%s", len(lines), count, buf.String())
	}
	seen := make(map[string]bool, count)
	for _, line := range lines {
		if !strings.HasPrefix(line, "[autodev] event-") {
			t.Fatalf("interleaved or malformed line %q", line)
		}
		seen[strings.TrimPrefix(line, "[autodev] ")] = true
	}
	for i := 0; i < count; i++ {
		if !seen[fmt.Sprintf("event-%d", i)] {
			t.Fatalf("missing complete terminal event %d", i)
		}
	}
}

func TestTerminalReporterRemoteEventRejectsUnsupportedKindWithoutConsumption(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminalReporter(&buf)
	event := RemoteEvent{EventID: "unknown:item", Kind: RemoteEventKind("unknown")}
	if err := rep.OnRemoteEvent(context.Background(), event); err == nil {
		t.Fatal("OnRemoteEvent accepted an unsupported event kind")
	}
	if buf.Len() != 0 {
		t.Fatalf("unsupported event output = %q", buf.String())
	}
	if _, ok := rep.delivered[event.EventID]; ok {
		t.Fatal("unsupported remote event was marked delivered")
	}
}
