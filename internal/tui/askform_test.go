package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

func key(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func formFor(questions ...tools.Question) *askForm {
	return newAskForm(askRequest{questions: questions, reply: make(chan answerResult, 1)})
}

func singleQ() tools.Question {
	return tools.Question{
		Prompt: "Pick one?",
		Options: []tools.Option{
			{Label: "Alpha", Description: "first", Preview: "ALPHA-PREVIEW"},
			{Label: "Beta", Description: "second"},
		},
	}
}

func TestAskFormSingleSelect(t *testing.T) {
	f := formFor(singleQ())
	f.update(key(tea.KeyDown)) // cursor -> Beta
	cmd := f.update(key(tea.KeyEnter))
	if !f.done || f.cancelled {
		t.Fatalf("expected done & not cancelled, got done=%v cancelled=%v", f.done, f.cancelled)
	}
	if len(f.answers) != 1 || f.answers[0].Value != "Beta" {
		t.Fatalf("unexpected answers: %+v", f.answers)
	}
	if cmd == nil {
		t.Fatal("expected askDoneCmd on completion")
	}
	if _, ok := cmd().(askDoneMsg); !ok {
		t.Fatal("completion cmd did not emit askDoneMsg")
	}
}

func TestAskFormSingleSelectCarriesPreview(t *testing.T) {
	f := formFor(singleQ())
	f.update(key(tea.KeyEnter)) // select Alpha (cursor 0)
	if f.answers[0].Preview != "ALPHA-PREVIEW" {
		t.Fatalf("expected preview carried into answer, got %q", f.answers[0].Preview)
	}
}

func TestAskFormMultiSelectJoins(t *testing.T) {
	q := tools.Question{
		Prompt:      "Pick many?",
		MultiSelect: true,
		Options: []tools.Option{
			{Label: "a"}, {Label: "b"}, {Label: "c"},
		},
	}
	f := formFor(q)
	f.update(key(tea.KeySpace)) // toggle a (cursor 0)
	f.update(key(tea.KeyDown))
	f.update(key(tea.KeyDown))  // cursor 2 (c)
	f.update(key(tea.KeySpace)) // toggle c
	f.update(key(tea.KeyEnter)) // confirm
	if len(f.answers) != 1 || f.answers[0].Value != "a, c" {
		t.Fatalf("expected joined 'a, c', got %+v", f.answers)
	}
}

func TestAskFormMultiSelectRequiresSelection(t *testing.T) {
	q := tools.Question{Prompt: "Pick many?", MultiSelect: true, Options: []tools.Option{{Label: "a"}, {Label: "b"}}}
	f := formFor(q)
	if cmd := f.update(key(tea.KeyEnter)); cmd != nil { // nothing toggled -> ignored
		t.Fatal("enter with no selection should be ignored")
	}
	if f.done || len(f.answers) != 0 {
		t.Fatalf("should not advance with no selection")
	}
}

func TestAskFormOtherFreeText(t *testing.T) {
	f := formFor(singleQ())
	f.update(key(tea.KeyDown)) // Beta
	f.update(key(tea.KeyDown)) // Other row (index 2)
	if !f.isOtherRow() {
		t.Fatalf("cursor should be on Other row, got %d", f.cursor)
	}
	f.update(key(tea.KeyEnter)) // enter other mode
	if !f.otherMode {
		t.Fatal("expected otherMode")
	}
	f.update(runes("DuckDB"))
	cmd := f.update(key(tea.KeyEnter))
	if len(f.answers) != 1 || f.answers[0].Value != "DuckDB" {
		t.Fatalf("expected free-text answer 'DuckDB', got %+v", f.answers)
	}
	if cmd == nil {
		t.Fatal("expected completion cmd")
	}
}

func TestAskFormOtherPreservesSpaces(t *testing.T) {
	// Regression (CODE-001): a space key must insert exactly one space.
	f := formFor(singleQ())
	f.update(key(tea.KeyDown))
	f.update(key(tea.KeyDown)) // Other row
	f.update(key(tea.KeyEnter))
	f.update(runes("two"))
	f.update(key(tea.KeySpace))
	f.update(runes("words"))
	f.update(key(tea.KeyEnter))
	if len(f.answers) != 1 || f.answers[0].Value != "two words" {
		t.Fatalf("expected 'two words', got %+v", f.answers)
	}
}

func TestAskFormCancel(t *testing.T) {
	f := formFor(singleQ())
	cmd := f.update(key(tea.KeyEsc))
	if !f.cancelled || !f.done {
		t.Fatalf("expected cancelled & done")
	}
	if _, ok := cmd().(askDoneMsg); !ok {
		t.Fatal("cancel should emit askDoneMsg")
	}
}

func TestAskFormTwoQuestionsOrdered(t *testing.T) {
	q1 := tools.Question{Prompt: "First?", Options: []tools.Option{{Label: "1a"}, {Label: "1b"}}}
	q2 := tools.Question{Prompt: "Second?", Options: []tools.Option{{Label: "2a"}, {Label: "2b"}}}
	f := formFor(q1, q2)
	f.update(key(tea.KeyEnter)) // q1 -> 1a, advance
	if f.done {
		t.Fatal("should not be done after first question")
	}
	cmd := f.update(key(tea.KeyEnter)) // q2 -> 2a, done
	if !f.done {
		t.Fatal("expected done after second question")
	}
	if len(f.answers) != 2 || f.answers[0].QuestionText != "First?" || f.answers[1].QuestionText != "Second?" {
		t.Fatalf("answers out of order: %+v", f.answers)
	}
	if _, ok := cmd().(askDoneMsg); !ok {
		t.Fatal("expected askDoneMsg")
	}
}

func TestAskFormViewRendersAndPreview(t *testing.T) {
	f := formFor(singleQ())
	out := f.view(80)
	if !strings.Contains(out, "Pick one?") || !strings.Contains(out, "Alpha") || !strings.Contains(out, otherOptionLabel) {
		t.Fatalf("view missing core content: %q", out)
	}
	if !strings.Contains(out, "ALPHA-PREVIEW") { // Alpha is focused at cursor 0
		t.Fatalf("preview not rendered for focused option: %q", out)
	}
}

func TestAskFormPreviewOnMultiSelectHarmless(t *testing.T) {
	q := tools.Question{
		Prompt:      "Pick many?",
		MultiSelect: true,
		Options:     []tools.Option{{Label: "a", Preview: "PREV-A"}, {Label: "b"}},
	}
	f := formFor(q)
	out := f.view(80) // must not panic; renders the option labels
	if !strings.Contains(out, "[ ] a") || !strings.Contains(out, "[ ] b") {
		t.Fatalf("multiSelect checkboxes missing: %q", out)
	}
}
