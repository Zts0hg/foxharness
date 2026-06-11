package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/autodev"
	tea "github.com/charmbracelet/bubbletea"
)

func TestAutodevBuiltinIsRegistered(t *testing.T) {
	found := false
	for _, cmd := range slashCommands {
		if cmd.Name == "/autodev" {
			found = true
		}
	}
	if !found {
		t.Fatal("/autodev is not in the builtin slashCommands list (REQ-025)")
	}
}

func TestAutodevCommandLaunchesOrchestrator(t *testing.T) {
	var gotBacklog string
	var gotReporter autodev.Reporter
	launched := make(chan struct{})

	m := NewModel(context.Background(), newFakeRunner(), Config{
		Autodev: func(ctx context.Context, backlogPath string, reporter autodev.Reporter) error {
			gotBacklog = backlogPath
			gotReporter = reporter
			close(launched)
			return nil
		},
	})

	next, cmd := m.handleSlashCommand("/autodev WORK.md")
	model := next.(Model)
	if !model.running {
		t.Error("model.running = false, want true while autodev runs")
	}
	if cmd == nil {
		t.Fatal("handleSlashCommand returned nil cmd, want autodev launch command")
	}

	msg := cmd()
	select {
	case <-launched:
	default:
		t.Fatal("autodev launcher was not invoked")
	}
	if gotBacklog != "WORK.md" {
		t.Errorf("backlog path = %q, want WORK.md", gotBacklog)
	}
	if _, ok := gotReporter.(*TUIReporter); !ok {
		t.Errorf("reporter = %T, want *TUIReporter (TC-017)", gotReporter)
	}
	if _, ok := msg.(runFinishedMsg); !ok {
		t.Errorf("completion msg = %T, want runFinishedMsg", msg)
	}
}

func TestAutodevCommandWithoutLauncherExplains(t *testing.T) {
	m := NewModel(context.Background(), newFakeRunner(), Config{})

	next, cmd := m.handleSlashCommand("/autodev")
	model := next.(Model)
	if cmd != nil {
		t.Error("cmd != nil, want no launch without a configured launcher")
	}
	if model.running {
		t.Error("model.running = true, want false")
	}
	last := model.entries[len(model.entries)-1]
	if !strings.Contains(strings.ToLower(last.body), "autodev") {
		t.Errorf("entry body = %q, want an explanation that autodev is unavailable", last.body)
	}
}

func TestAutodevCommandRefusesWhileRunning(t *testing.T) {
	m := NewModel(context.Background(), newFakeRunner(), Config{
		Autodev: func(ctx context.Context, backlogPath string, reporter autodev.Reporter) error { return nil },
	})
	m.running = true

	next, cmd := m.handleSlashCommand("/autodev")
	if cmd != nil {
		t.Error("cmd != nil, want refusal while a run is active")
	}
	if !strings.Contains(next.(Model).status, "run") {
		t.Errorf("status = %q, want busy explanation", next.(Model).status)
	}
}

var _ = tea.Quit
