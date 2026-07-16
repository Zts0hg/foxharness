package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEffortFormSelectsOption(t *testing.T) {
	form := newEffortForm("openai", []string{"auto", "none", "minimal", "low"}, "minimal")
	if form.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", form.cursor)
	}
	cmd := form.update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter returned nil cmd")
	}
	if _, ok := cmd().(effortDoneMsg); !ok {
		t.Fatal("cmd did not return effortDoneMsg")
	}
	if form.result != "minimal" {
		t.Fatalf("result = %q, want minimal", form.result)
	}
}

func TestEffortFormEscapeCancels(t *testing.T) {
	form := newEffortForm("claude", []string{"auto", "low", "medium"}, "low")
	cmd := form.update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc returned nil cmd")
	}
	if _, ok := cmd().(effortDoneMsg); !ok {
		t.Fatal("cmd did not return effortDoneMsg")
	}
	if form.result != "" {
		t.Fatalf("result = %q, want empty cancel result", form.result)
	}
}
