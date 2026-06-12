package tui

import (
	"context"
	"strings"
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
