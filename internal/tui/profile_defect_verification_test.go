package tui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/slash"
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
