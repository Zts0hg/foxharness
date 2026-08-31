package tui

import (
	"context"
	"errors"
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

func TestUIAUT003AutodevRunLifecycleAndCancellation(t *testing.T) {
	started := make(chan context.Context, 1)
	m := NewModel(context.Background(), newFakeRunner(), Config{
		Autodev: func(ctx context.Context, backlogPath string, reporter autodev.Reporter) error {
			if backlogPath != "WORK.md" {
				t.Errorf("backlog path = %q, want WORK.md", backlogPath)
			}
			started <- ctx
			<-ctx.Done()
			return ctx.Err()
		},
	})

	next, cmd := m.handleSlashCommand("/autodev WORK.md")
	running := next.(Model)
	if !running.running || running.cancelRun == nil || running.status != "autodev running" || running.activeOperationID == 0 {
		t.Fatalf("running state = running:%t cancel:%t status:%q operation:%d", running.running, running.cancelRun != nil, running.status, running.activeOperationID)
	}
	startEntry := running.entries[len(running.entries)-1]
	if startEntry.title != "Autodev" || !strings.Contains(startEntry.body, "Draining the backlog autonomously") {
		t.Fatalf("start entry = %+v", startEntry)
	}

	finished := make(chan tea.Msg, 1)
	go func() { finished <- cmd() }()
	runCtx := <-started
	select {
	case <-runCtx.Done():
		t.Fatal("autodev context cancelled before an explicit cancel command")
	default:
	}

	cancelledModel, cancelCmd := running.handleSlashCommand("/cancel")
	cancelling := cancelledModel.(Model)
	if cancelCmd != nil || cancelling.status != "Cancel requested" {
		t.Fatalf("cancel result = cmd:%v status:%q", cancelCmd, cancelling.status)
	}
	msg := (<-finished).(runFinishedMsg)
	if !errors.Is(msg.err, context.Canceled) || msg.operationID != running.activeOperationID {
		t.Fatalf("finished message = %+v", msg)
	}

	updated, _ := cancelling.Update(msg)
	done := updated.(Model)
	if done.running || done.cancelRun != nil || done.activeOperationID != 0 || done.status != "Conversation interrupted" {
		t.Fatalf("finished state = running:%t cancel:%t status:%q operation:%d", done.running, done.cancelRun != nil, done.status, done.activeOperationID)
	}
	entryCount := len(done.entries)
	updated, _ = done.Update(msg)
	duplicate := updated.(Model)
	if duplicate.running || len(duplicate.entries) != entryCount || duplicate.status != done.status {
		t.Fatalf("duplicate finish changed state: before entries/status %d/%q after %d/%q", entryCount, done.status, len(duplicate.entries), duplicate.status)
	}
}

func TestUIAUT003AutodevCompletionRestoresStateExactlyOnce(t *testing.T) {
	for _, tc := range []struct {
		name       string
		runErr     error
		wantStatus string
	}{
		{name: "success", wantStatus: "Run complete"},
		{name: "ordinary failure", runErr: errors.New("pipeline failed"), wantStatus: "Run failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewModel(context.Background(), newFakeRunner(), Config{
				Autodev: func(context.Context, string, autodev.Reporter) error { return tc.runErr },
			})
			next, cmd := m.handleSlashCommand("/autodev WORK.md")
			running := next.(Model)
			msg := cmd().(runFinishedMsg)
			updated, _ := running.Update(msg)
			done := updated.(Model)
			if done.running || done.cancelRun != nil || done.activeOperationID != 0 || done.status != tc.wantStatus {
				t.Fatalf("finished state = running:%t cancel:%t status:%q operation:%d", done.running, done.cancelRun != nil, done.status, done.activeOperationID)
			}
			entryCount := len(done.entries)
			updated, _ = done.Update(msg)
			duplicate := updated.(Model)
			if duplicate.status != done.status || len(duplicate.entries) != entryCount {
				t.Fatalf("duplicate completion changed state: before %q/%d after %q/%d", done.status, entryCount, duplicate.status, len(duplicate.entries))
			}
		})
	}
}

var _ = tea.Quit
