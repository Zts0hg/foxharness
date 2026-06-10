package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

func TestModelOpensRoutesAndCompletesAskForm(t *testing.T) {
	runner := newFakeRunner()
	asker := NewAsker()
	m := NewModel(context.Background(), runner, Config{Asker: asker})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	reply := make(chan answerResult, 1)
	req := askRequest{
		questions: []tools.Question{{Prompt: "Pick?", Options: []tools.Option{{Label: "A"}, {Label: "B"}}}},
		reply:     reply,
	}

	// askUserMsg opens the overlay.
	m, _ = update(t, m, askUserMsg{req: req})
	if m.askForm == nil {
		t.Fatal("askForm should be open after askUserMsg")
	}

	// View renders the overlay.
	if !strings.Contains(m.View(), "Pick?") {
		t.Fatalf("overlay not rendered in View: %q", m.View())
	}

	// Keys route to the overlay, not the prompt input.
	m, _ = update(t, m, keyRunes("z"))
	if len(m.input) != 0 {
		t.Fatalf("keys leaked to prompt input: %q", string(m.input))
	}

	// Enter selects the focused option (A) and completes the single-question form.
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("expected a command after overlay completion")
	}
	done := cmd()
	if _, ok := done.(askDoneMsg); !ok {
		t.Fatalf("expected askDoneMsg, got %T", done)
	}

	// Delivering askDoneMsg replies to the engine and clears the overlay.
	m, _ = update(t, m, done)
	if m.askForm != nil {
		t.Fatal("overlay should be cleared after askDoneMsg")
	}
	select {
	case res := <-reply:
		if res.cancelled || len(res.answers) != 1 || res.answers[0].Value != "A" {
			t.Fatalf("unexpected reply: %+v", res)
		}
	default:
		t.Fatal("model did not reply on the request channel")
	}
}

func TestAskFormRendersInlineWithTranscript(t *testing.T) {
	// The question card must render at the bottom while the conversation
	// transcript stays visible above it — not a full-screen takeover.
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{Asker: NewAsker()})
	m.entries = nil
	m.appendEntry("assistant", "", "TRANSCRIPT_MARKER_42", false)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	reply := make(chan answerResult, 1)
	req := askRequest{questions: []tools.Question{{Prompt: "Pick?", Options: []tools.Option{{Label: "A"}, {Label: "B"}}}}, reply: reply}
	m, _ = update(t, m, askUserMsg{req: req})

	out := m.View()
	if !strings.Contains(out, "TRANSCRIPT_MARKER_42") {
		t.Fatalf("transcript should remain visible while the question is shown:\n%s", out)
	}
	if !strings.Contains(out, "Pick?") {
		t.Fatalf("question card should be rendered:\n%s", out)
	}
}

func TestModelAskFormCancelReplies(t *testing.T) {
	runner := newFakeRunner()
	asker := NewAsker()
	m := NewModel(context.Background(), runner, Config{Asker: asker})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	reply := make(chan answerResult, 1)
	req := askRequest{questions: []tools.Question{{Prompt: "Pick?", Options: []tools.Option{{Label: "A"}, {Label: "B"}}}}, reply: reply}
	m, _ = update(t, m, askUserMsg{req: req})

	m, cmd := update(t, m, key(tea.KeyEsc))
	if _, ok := cmd().(askDoneMsg); !ok {
		t.Fatalf("esc should produce askDoneMsg")
	}
	m, _ = update(t, m, askDoneMsg{})
	select {
	case res := <-reply:
		if !res.cancelled {
			t.Fatalf("expected cancelled reply, got %+v", res)
		}
	default:
		t.Fatal("model did not reply on cancellation")
	}
	if m.askForm != nil {
		t.Fatal("overlay should be cleared after cancel")
	}
}
