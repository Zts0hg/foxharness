package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/slash"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPFTUI017CancellationStopsPromptCommandPreparation(t *testing.T) {
	workDir := t.TempDir()
	marker := filepath.Join(workDir, "must-not-exist")
	runner := newFakeRunner()
	runner.workDir = workDir
	registry := newRegistryWithPromptCommand(t, "prepare", "Inspect !`touch "+marker+"`")
	m := NewModel(context.Background(), runner, Config{}).WithRegistry(registry, slash.NewExecutor(slash.WithWorkDir(workDir)))

	m, _ = update(t, m, keyRunes("/prepare"))
	m, prepareCmd := update(t, m, keyEnter())
	if prepareCmd == nil || !m.running || m.cancelRun == nil {
		t.Fatalf("prompt preparation did not become cancellable: running=%v cancel=%v cmd=%v", m.running, m.cancelRun != nil, prepareCmd)
	}
	m, _ = update(t, m, keyCtrlC())
	m, _ = update(t, m, prepareCmd())
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("cancelled prompt preparation created marker: %v", err)
	}
	if m.running || m.cancelRun != nil {
		t.Fatalf("cancelled prompt preparation retained active state: running=%v cancel=%v", m.running, m.cancelRun != nil)
	}
}

func TestPFTUI017StaleCompletionCannotTerminateLaterRun(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("first task"))
	m, firstCmd := update(t, m, keyEnter())
	if firstCmd == nil {
		t.Fatal("first task command is nil")
	}
	m, _ = update(t, m, keyRunes("second task"))
	m, _ = update(t, m, keyEnter())

	firstCompletion := firstCmd()
	m, secondCmd := update(t, m, firstCompletion)
	if secondCmd == nil || !m.running {
		t.Fatalf("first completion did not start the queued second run: running=%v cmd=%v", m.running, secondCmd)
	}

	m, staleCmd := update(t, m, firstCompletion)
	if staleCmd != nil {
		t.Fatal("stale first-run completion dispatched unexpected work")
	}
	if !m.running {
		t.Fatal("stale first-run completion terminated the active second run")
	}
	status := m.status
	m, _ = update(t, m, runEventMsg{operationID: 1, role: "assistant", body: "stale response", status: "stale"})
	if m.status != status || entriesContainText(m.entries, "assistant", "stale response") {
		t.Fatal("stale first-run event mutated the active second run")
	}
}

func TestPFTUI017CancellationClosesRunInteractionBeforeContinuingQueue(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true
	m.queuedPrompts = testQueuedPrompts("queued after cancellation")
	m.askForm = newAskForm(askRequest{
		request: app.QuestionRequest{Questions: []app.Question{{Prompt: "Continue?", Options: []app.QuestionOption{{Label: "Yes"}, {Label: "No"}}}}},
		reply:   make(chan answerResult, 1),
	})

	m, queuedCmd := update(t, m, runFinishedMsg{err: context.Canceled})
	if m.askForm != nil {
		t.Fatal("cancelled run retained a stale question overlay")
	}
	if queuedCmd == nil || !m.running {
		t.Fatalf("cancelled run did not continue queued work: running=%v cmd=%v", m.running, queuedCmd)
	}
	if len(m.queuedPrompts) != 0 {
		t.Fatalf("queued prompts = %#v, want dispatched", m.queuedPrompts)
	}
}

func TestPFTUI017CancellationResolvesEveryPendingRunInteraction(t *testing.T) {
	t.Run("plan review", func(t *testing.T) {
		m := NewModel(context.Background(), newFakeRunner(), Config{})
		reply := make(chan planReviewResult, 1)
		m.running = true
		m.planForm = newPlanReviewForm(planReviewRequest{request: app.PlanReviewRequest{PlanMarkdown: "# Plan"}, reply: reply})

		m, _ = update(t, m, runFinishedMsg{err: context.Canceled})
		if m.planForm != nil {
			t.Fatal("cancelled run retained a stale plan-review overlay")
		}
		select {
		case result := <-reply:
			if !result.cancelled {
				t.Fatalf("plan-review cancellation = %#v", result)
			}
		default:
			t.Fatal("cancelled run did not resolve the pending plan review")
		}
	})

	t.Run("approval", func(t *testing.T) {
		m := NewModel(context.Background(), newFakeRunner(), Config{})
		reply := make(chan app.PermissionResponse, 1)
		m.running = true
		m.approvalForm = newApprovalForm(permissionRequest{reply: reply})

		m, _ = update(t, m, runFinishedMsg{err: context.Canceled})
		if m.approvalForm != nil {
			t.Fatal("cancelled run retained a stale approval overlay")
		}
		select {
		case decision := <-reply:
			if decision.Decision != app.PermissionDeny {
				t.Fatalf("approval cancellation = %#v, want deny", decision)
			}
		default:
			t.Fatal("cancelled run did not resolve the pending approval")
		}
	})
}

func TestPFTUI017CancellationRearmsConsumedInteractionListeners(t *testing.T) {
	tests := []struct {
		name  string
		model func() Model
	}{
		{
			name: "question",
			model: func() Model {
				m := NewModel(context.Background(), newFakeRunner(), Config{Asker: NewAsker()})
				m.askForm = newAskForm(askRequest{reply: make(chan answerResult, 1)})
				return m
			},
		},
		{
			name: "plan review",
			model: func() Model {
				m := NewModel(context.Background(), newFakeRunner(), Config{PlanReviewer: NewPlanReviewer()})
				m.planForm = newPlanReviewForm(planReviewRequest{reply: make(chan planReviewResult, 1)})
				return m
			},
		},
		{
			name: "permission",
			model: func() Model {
				m := NewModel(context.Background(), newFakeRunner(), Config{Permissions: NewPermissionBridge()})
				m.approvalForm = newApprovalForm(permissionRequest{reply: make(chan app.PermissionResponse, 1)})
				return m
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.model()
			m.running = true
			m, rearm := update(t, m, runFinishedMsg{err: context.Canceled})
			if m.hasBlockingInteraction() {
				t.Fatal("cancelled run retained a blocking interaction")
			}
			if rearm == nil {
				t.Fatal("cancelled run did not re-arm the consumed interaction listener")
			}
		})
	}
}

func TestPFTUI017RearmedInteractionListenersAcceptNextCorrelatedRequest(t *testing.T) {
	t.Run("question", func(t *testing.T) {
		asker := NewAsker()
		lifetimeCtx, stop := context.WithCancel(context.Background())
		defer stop()
		m := NewModel(lifetimeCtx, newFakeRunner(), Config{Asker: asker})
		m.running = true
		m.askForm = newAskForm(askRequest{reply: make(chan answerResult, 1)})
		_, rearm := update(t, m, runFinishedMsg{err: context.Canceled})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := asker.AskQuestions(ctx, app.QuestionRequest{Correlation: app.InteractionCorrelation{ID: "question-next"}})
			done <- err
		}()
		msg := runCommandWithTimeout(t, rearm)
		request, ok := msg.(askUserMsg)
		if !ok || request.req.request.Correlation.ID != "question-next" {
			t.Fatalf("rearmed question message = %#v", msg)
		}
		cancel()
		waitForContextCancellation(t, done)
	})

	t.Run("plan review", func(t *testing.T) {
		reviewer := NewPlanReviewer()
		lifetimeCtx, stop := context.WithCancel(context.Background())
		defer stop()
		m := NewModel(lifetimeCtx, newFakeRunner(), Config{PlanReviewer: reviewer})
		m.running = true
		m.planForm = newPlanReviewForm(planReviewRequest{reply: make(chan planReviewResult, 1)})
		_, rearm := update(t, m, runFinishedMsg{err: context.Canceled})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := reviewer.ReviewPlan(ctx, app.PlanReviewRequest{Correlation: app.InteractionCorrelation{ID: "plan-next"}})
			done <- err
		}()
		msg := runCommandWithTimeout(t, rearm)
		request, ok := msg.(planReviewMsg)
		if !ok || request.req.request.Correlation.ID != "plan-next" {
			t.Fatalf("rearmed plan-review message = %#v", msg)
		}
		cancel()
		waitForContextCancellation(t, done)
	})

	t.Run("permission", func(t *testing.T) {
		bridge := NewPermissionBridge()
		lifetimeCtx, stop := context.WithCancel(context.Background())
		defer stop()
		m := NewModel(lifetimeCtx, newFakeRunner(), Config{Permissions: bridge})
		m.running = true
		m.approvalForm = newApprovalForm(permissionRequest{reply: make(chan app.PermissionResponse, 1)})
		_, rearm := update(t, m, runFinishedMsg{err: context.Canceled})

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			_, err := bridge.RequestPermission(ctx, app.PermissionRequest{Correlation: app.InteractionCorrelation{ID: "permission-next"}})
			done <- err
		}()
		msg := runCommandWithTimeout(t, rearm)
		request, ok := msg.(permissionUserMsg)
		if !ok || request.req.approval.Correlation.ID != "permission-next" {
			t.Fatalf("rearmed permission message = %#v", msg)
		}
		cancel()
		waitForContextCancellation(t, done)
	})
}

func runCommandWithTimeout(t *testing.T, command tea.Cmd) tea.Msg {
	t.Helper()
	messages := make(chan tea.Msg, 1)
	go func() { messages <- command() }()
	select {
	case msg := <-messages:
		return msg
	case <-time.After(time.Second):
		t.Fatal("rearmed listener did not accept the next request")
		return nil
	}
}

func waitForContextCancellation(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("interaction cancellation = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("next interaction did not stop after context cancellation")
	}
}

func TestPFTUI005QueueWaitsWhileBlockingOverlayOwnsInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true
	m.queuedPrompts = testQueuedPrompts("queued after interaction")
	m.askForm = newAskForm(askRequest{
		request: app.QuestionRequest{Questions: []app.Question{{Prompt: "Continue?", Options: []app.QuestionOption{{Label: "Yes"}, {Label: "No"}}}}},
		reply:   make(chan answerResult, 1),
	})

	m, queuedCmd := update(t, m, runFinishedMsg{result: &app.RunOutcome{RunID: "run-with-overlay"}})
	if queuedCmd != nil {
		t.Fatal("run completion dispatched queued work while the question overlay owned input")
	}
	if len(m.queuedPrompts) != 1 {
		t.Fatalf("queued prompts = %#v, want deferred prompt", m.queuedPrompts)
	}
	if m.askForm == nil {
		t.Fatal("successful completion discarded the unresolved interaction overlay")
	}

	m, doneCmd := update(t, m, keyEnter())
	if doneCmd == nil {
		t.Fatal("question response did not produce a completion command")
	}
	m, queuedCmd = update(t, m, doneCmd())
	if queuedCmd == nil || !m.running || len(m.queuedPrompts) != 0 {
		t.Fatalf("resolved overlay did not dispatch deferred queue: running=%v queued=%#v cmd=%v", m.running, m.queuedPrompts, queuedCmd)
	}
}
