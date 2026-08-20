package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/autodev"
	tea "github.com/charmbracelet/bubbletea"
)

func collectEvent(t *testing.T, events chan tea.Msg) runEventMsg {
	t.Helper()
	select {
	case msg := <-events:
		ev, ok := msg.(runEventMsg)
		if !ok {
			t.Fatalf("event type = %T, want runEventMsg", msg)
		}
		return ev
	case <-time.After(time.Second):
		t.Fatal("no event arrived on the channel")
		return runEventMsg{}
	}
}

func TestTUIReporterForwardsOrchestrationEvents(t *testing.T) {
	events := make(chan tea.Msg, 16)
	rep := NewTUIReporter(events)
	ctx := context.Background()

	rep.OnItemStart(ctx, 1, 2, autodev.LedgerItem{Slug: "engine-memory", Priority: autodev.PriorityHigh})
	ev := collectEvent(t, events)
	if !strings.Contains(ev.body, "engine-memory") || !strings.Contains(ev.body, "1/2") {
		t.Errorf("OnItemStart body = %q, want item progress rendered (TC-017)", ev.body)
	}

	rep.OnStageStart(ctx, "engine-memory", "generate-spec")
	ev = collectEvent(t, events)
	if !strings.Contains(ev.body, "generate-spec") {
		t.Errorf("OnStageStart body = %q, want stage name", ev.body)
	}

	rep.OnVerify(ctx, "generate-spec", false, "spec.md absent")
	ev = collectEvent(t, events)
	if !strings.Contains(ev.body, "spec.md absent") {
		t.Errorf("OnVerify body = %q, want the gap", ev.body)
	}

	rep.OnEngineerReview(ctx, "generate-spec", "write the spec now")
	ev = collectEvent(t, events)
	if !strings.Contains(ev.body, "write the spec now") {
		t.Errorf("OnEngineerReview body = %q, want the correction", ev.body)
	}

	rep.OnIssue(ctx, 31)
	ev = collectEvent(t, events)
	if !strings.Contains(ev.body, "#31") {
		t.Errorf("OnIssue body = %q, want issue number", ev.body)
	}

	rep.OnPR(ctx, 32)
	ev = collectEvent(t, events)
	if !strings.Contains(ev.body, "#32") {
		t.Errorf("OnPR body = %q, want PR number", ev.body)
	}

	rep.OnItemDone(ctx, autodev.LedgerItem{Slug: "engine-memory", Issue: 31, PR: 32})
	ev = collectEvent(t, events)
	if !strings.Contains(ev.body, "engine-memory") {
		t.Errorf("OnItemDone body = %q, want slug", ev.body)
	}
}

func TestTUIReporterForwardsEngineEvents(t *testing.T) {
	events := make(chan tea.Msg, 16)
	rep := NewTUIReporter(events)

	rep.OnToolCall(context.Background(), "bash", `{"command":"git add -A"}`)
	ev := collectEvent(t, events)
	if ev.role != "tool" {
		t.Errorf("OnToolCall role = %q, want tool (same stream as normal runs)", ev.role)
	}
}

func TestUIAUT004ReporterMapsCompleteOrderedEventStream(t *testing.T) {
	events := make(chan tea.Msg, 32)
	rep := newTUIReporterForOperation(events, 42)
	ctx := context.Background()

	rep.OnRunStart(ctx, "session-1", "run-1")
	rep.OnThinking(ctx, 2)
	rep.OnCompaction(ctx, "automatic")
	rep.OnToolCall(ctx, "bash", `{"command":"git status"}`)
	rep.OnToolResult(ctx, "bash", " failed ", true)
	rep.OnToolResult(ctx, "bash", " ok ", false)
	rep.OnMessage(ctx, " plain ")
	rep.OnMessageDelta(ctx, "part")
	rep.OnMessage(ctx, " final ")
	rep.OnRunComplete(ctx, autodev.CoreRunResult{RunID: "run-1"})
	rep.OnRunError(ctx, "session-1", "run-1", errors.New("model failed"))
	rep.OnItemStart(ctx, 1, 2, autodev.LedgerItem{Slug: "item", Priority: autodev.PriorityHigh})
	rep.OnWorktree(ctx, autodev.Worktree{Path: "/tmp/item", Branch: "auto/item"})
	rep.OnStageStart(ctx, "item", "generate-spec")
	rep.OnEngineerDecision(ctx, []autodev.Question{{Prompt: "Proceed?"}}, []autodev.Answer{{Value: "Yes"}})
	rep.OnEngineerReview(ctx, "generate-spec", "write spec")
	rep.OnVerify(ctx, "generate-spec", true, "")
	rep.OnVerify(ctx, "generate-spec", false, "spec missing")
	rep.OnGate(ctx, autodev.GateResult{Steps: []autodev.GateStep{
		{Name: "build", Passed: true},
		{Name: "test"},
		{Name: "gofmt", Skipped: true},
	}})
	rep.OnIssue(ctx, 30)
	if err := rep.OnRemoteEvent(ctx, autodev.RemoteEvent{EventID: "issue:item:31", Kind: autodev.RemoteEventIssue, Number: 31}); err != nil {
		t.Fatal(err)
	}
	rep.OnPR(ctx, 32)
	rep.OnItemDone(ctx, autodev.LedgerItem{Slug: "item", Issue: 31, PR: 32})
	rep.OnInfo(ctx, "WARNING: failed to remove worktree: busy")

	want := []runEventMsg{
		{operationID: 42, status: "Run started: run-1"},
		{operationID: 42, status: "Thinking turn 2"},
		{operationID: 42, role: "system", title: "context compacted", body: "Compacted context scope: automatic", status: "Context compacted"},
		{operationID: 42, role: "tool", title: "call bash", body: "Bash (git status)", status: "Calling tool: bash"},
		{operationID: 42, role: "tool", title: "result bash", body: "failed", status: "Tool failed: bash", err: true},
		{operationID: 42, role: "tool", title: "result bash", body: "ok", status: "Tool complete: bash"},
		{operationID: 42, role: "assistant", title: "foxharness", body: "plain", status: "Assistant responded"},
		{operationID: 42, role: "assistant", title: "stream", body: "part", status: "Assistant responding", delta: true},
		{operationID: 42, role: "assistant", title: "foxharness", body: "final", status: "Assistant responded", streamFinal: true},
		{operationID: 42, status: "Run complete: run-1"},
		{operationID: 42, role: "error", title: "run error", body: "Session: session-1\nRun: run-1\nError: model failed", status: "Run failed", err: true},
		{operationID: 42, role: "system", title: "autodev", body: "item 1/2  high  item", status: "autodev: item"},
		{operationID: 42, role: "system", title: "autodev", body: "worktree /tmp/item  branch auto/item"},
		{operationID: 42, role: "system", title: "autodev stage", body: "generate-spec", status: "autodev stage: generate-spec"},
		{operationID: 42, role: "system", title: "engineer decision", body: "core asks: Proceed?\nengineer → Yes"},
		{operationID: 42, role: "system", title: "engineer review", body: "write spec", status: "engineer steering generate-spec"},
		{operationID: 42, role: "system", title: "autodev verify", body: "generate-spec  DONE"},
		{operationID: 42, role: "system", title: "autodev verify", body: "generate-spec  NOT DONE: spec missing"},
		{operationID: 42, role: "system", title: "autodev gate", body: "build ✓  test ✗  gofmt skipped"},
		{operationID: 42, role: "system", title: "autodev remote", body: "issue #30"},
		{operationID: 42, role: "system", title: "autodev remote", body: "issue #31"},
		{operationID: 42, role: "system", title: "autodev remote", body: "PR #32"},
		{operationID: 42, role: "system", title: "autodev", body: "item = done (issue #31, pr #32)", status: "autodev: item done"},
		{operationID: 42, role: "system", title: "autodev", body: "WARNING: failed to remove worktree: busy"},
	}
	for i, expected := range want {
		if got := collectEvent(t, events); !reflect.DeepEqual(got, expected) {
			t.Fatalf("event %d = %#v, want %#v", i, got, expected)
		}
	}
	if len(events) != 0 {
		t.Fatalf("unexpected trailing events = %d", len(events))
	}
}

func TestUIAUT006TerminalAndTUIConsumeSameTypedControlFacts(t *testing.T) {
	var terminal bytes.Buffer
	terminalReporter := autodev.NewTerminalReporter(&terminal)
	tuiEvents := make(chan tea.Msg, 16)
	tuiReporter := newTUIReporterForOperation(tuiEvents, 91)

	emit := func(reporter autodev.Reporter) {
		ctx := context.Background()
		reporter.OnItemStart(ctx, 1, 1, autodev.LedgerItem{Slug: "item", Priority: autodev.PriorityHigh})
		reporter.OnWorktree(ctx, autodev.Worktree{Path: "/tmp/item", Branch: "auto/item"})
		reporter.OnStageStart(ctx, "item", "generate-spec")
		reporter.OnEngineerDecision(ctx, []autodev.Question{{Prompt: "Proceed?"}}, []autodev.Answer{{Value: "Yes"}})
		reporter.OnEngineerReview(ctx, "generate-spec", "write spec")
		reporter.OnVerify(ctx, "generate-spec", false, "spec missing")
		reporter.OnGate(ctx, autodev.GateResult{Steps: []autodev.GateStep{{Name: "test", Passed: true}}})
		reporter.OnIssue(ctx, 31)
		reporter.OnPR(ctx, 32)
		reporter.OnItemDone(ctx, autodev.LedgerItem{Slug: "item", Issue: 31, PR: 32})
		reporter.OnInfo(ctx, "backlog drained")
	}
	emit(terminalReporter)
	emit(tuiReporter)

	for _, fact := range []string{"item", "/tmp/item", "generate-spec", "Proceed?", "Yes", "write spec", "spec missing", "test", "#31", "#32", "backlog drained"} {
		if !strings.Contains(terminal.String(), fact) {
			t.Fatalf("terminal adapter omitted typed fact %q:\n%s", fact, terminal.String())
		}
	}
	var tuiBody strings.Builder
	for i := 0; i < 11; i++ {
		event := collectEvent(t, tuiEvents)
		if event.operationID != 91 {
			t.Fatalf("TUI event operation ID = %d, want 91", event.operationID)
		}
		tuiBody.WriteString(event.body)
		tuiBody.WriteByte('\n')
	}
	for _, fact := range []string{"item", "/tmp/item", "generate-spec", "Proceed?", "Yes", "write spec", "spec missing", "test", "#31", "#32", "backlog drained"} {
		if !strings.Contains(tuiBody.String(), fact) {
			t.Fatalf("TUI adapter omitted typed fact %q:\n%s", fact, tuiBody.String())
		}
	}
}

func TestUIAUT005TUIReporterSerializesConcurrentDeliveries(t *testing.T) {
	const count = 64
	events := make(chan tea.Msg, count)
	rep := newTUIReporterForOperation(events, 77)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rep.OnInfo(context.Background(), fmt.Sprintf("event-%d", i))
		}(i)
	}
	wg.Wait()

	seen := make(map[string]bool, count)
	for i := 0; i < count; i++ {
		event := collectEvent(t, events)
		if event.operationID != 77 || event.role != "system" || event.title != "autodev" {
			t.Fatalf("concurrent event = %#v", event)
		}
		seen[event.body] = true
	}
	for i := 0; i < count; i++ {
		if !seen[fmt.Sprintf("event-%d", i)] {
			t.Fatalf("missing complete concurrent event %d", i)
		}
	}
}

func TestTUIReporterConsumesRemoteEventIDIdempotently(t *testing.T) {
	events := make(chan tea.Msg, 2)
	rep := NewTUIReporter(events)
	event := autodev.RemoteEvent{
		EventID: "issue:item-tui:31",
		ItemID:  "item-tui",
		Kind:    autodev.RemoteEventIssue,
		Number:  31,
	}
	if err := rep.OnRemoteEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if err := rep.OnRemoteEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	if got := len(events); got != 1 {
		t.Fatalf("remote event messages = %d, want one for duplicate EventID", got)
	}
	if body := collectEvent(t, events).body; !strings.Contains(body, "#31") {
		t.Errorf("remote event body = %q, want issue number", body)
	}
}

func TestTUIReporterRemoteEventReturnsCancellationBeforeConsumption(t *testing.T) {
	events := make(chan tea.Msg)
	rep := NewTUIReporter(events)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	event := autodev.RemoteEvent{EventID: "issue:item-tui:31", ItemID: "item-tui", Kind: autodev.RemoteEventIssue, Number: 31}
	if err := rep.OnRemoteEvent(ctx, event); err == nil {
		t.Fatal("OnRemoteEvent returned nil for canceled delivery")
	}
}

func TestTUIReporterRemoteEventRejectsUnsupportedKindWithoutConsumption(t *testing.T) {
	events := make(chan tea.Msg, 1)
	rep := NewTUIReporter(events)
	event := autodev.RemoteEvent{EventID: "unknown:item", Kind: autodev.RemoteEventKind("unknown")}
	if err := rep.OnRemoteEvent(context.Background(), event); err == nil {
		t.Fatal("OnRemoteEvent accepted an unsupported event kind")
	}
	if len(events) != 0 {
		t.Fatal("unsupported remote event was delivered")
	}
	if _, ok := rep.deliveredRemote[event.EventID]; ok {
		t.Fatal("unsupported remote event was marked delivered")
	}
}

func TestTUIReporterDoesNotBlockOnCancelledContext(t *testing.T) {
	events := make(chan tea.Msg) // unbuffered and never drained
	rep := NewTUIReporter(events)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		rep.OnItemStart(ctx, 1, 1, autodev.LedgerItem{Slug: "x"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reporter blocked on a cancelled context, want non-blocking return")
	}
}

func TestTUIReporterImplementsAutodevReporter(t *testing.T) {
	var _ autodev.Reporter = NewTUIReporter(make(chan tea.Msg))
}
