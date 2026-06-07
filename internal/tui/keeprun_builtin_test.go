package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Zts0hg/foxharness/internal/keeprun"
)

func TestNewTUIPhaseRunnerWiring(t *testing.T) {
	events := make(chan tea.Msg, 1)
	pr := newTUIPhaseRunner(&fakeEngine{}, nil, nil, events)

	if _, ok := pr.reporter.(*channelReporter); !ok {
		t.Errorf("reporter = %T, want *channelReporter so within-phase output streams to the TUI", pr.reporter)
	}
	if pr.onPrompt == nil {
		t.Fatal("onPrompt not wired; the per-phase header would not be shown")
	}
	pr.onPrompt(context.Background(), "/codexspec:specify\n\nINJECTED-INSTRUCTION")
	msg := <-events
	ev, ok := msg.(runEventMsg)
	if !ok {
		t.Fatalf("got %T, want runEventMsg", msg)
	}
	if ev.role != "user" {
		t.Errorf("header role = %q, want user", ev.role)
	}
	if !strings.Contains(ev.body, "/codexspec:specify") || !strings.Contains(ev.body, "INJECTED-INSTRUCTION") {
		t.Errorf("header body = %q", ev.body)
	}
}

func TestEventSinkPostsProgress(t *testing.T) {
	ch := make(chan tea.Msg, 1)
	eventSink{events: ch}.Event(keeprun.ProgressEvent{
		Kind: keeprun.EventPhaseStart, Phase: 3, Total: 12, Command: "codexspec:generate-spec",
	})
	msg := <-ch
	pm, ok := msg.(keepRunProgressMsg)
	if !ok {
		t.Fatalf("got %T, want keepRunProgressMsg", msg)
	}
	if pm.event.Command != "codexspec:generate-spec" {
		t.Errorf("event command = %q", pm.event.Command)
	}
}

func TestFormatKeepRunEvent(t *testing.T) {
	cases := []struct {
		ev   keeprun.ProgressEvent
		want []string
	}{
		{keeprun.ProgressEvent{Kind: keeprun.EventPhaseStart, Phase: 3, Total: 12, Command: "codexspec:generate-spec"}, []string{"Phase 3/12", "generate-spec"}},
		{keeprun.ProgressEvent{Kind: keeprun.EventPhaseComplete, Phase: 11, Total: 12, Command: "codexspec:commit-staged"}, []string{"complete", "commit-staged"}},
		{keeprun.ProgressEvent{Kind: keeprun.EventExit, Message: "All tasks completed"}, []string{"All tasks completed"}},
	}
	for _, c := range cases {
		got := formatKeepRunEvent(c.ev)
		for _, sub := range c.want {
			if !strings.Contains(got, sub) {
				t.Errorf("formatKeepRunEvent(%v) = %q, want substring %q", c.ev.Kind, got, sub)
			}
		}
	}
}
