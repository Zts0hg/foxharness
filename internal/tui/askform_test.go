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
	f.update(key(tea.KeyDown)) // free-text row (index 2)
	if !f.onOtherRow() {
		t.Fatalf("cursor should be on the free-text row, got %d", f.cursor)
	}
	f.update(runes("DuckDB")) // type directly — no Enter needed to start
	cmd := f.update(key(tea.KeyEnter))
	if len(f.answers) != 1 || f.answers[0].Value != "DuckDB" {
		t.Fatalf("expected free-text answer 'DuckDB', got %+v", f.answers)
	}
	if cmd == nil {
		t.Fatal("expected completion cmd")
	}
}

func TestAskFormOtherPreservesSpaces(t *testing.T) {
	// A space key must insert exactly one space.
	f := formFor(singleQ())
	f.update(key(tea.KeyDown))
	f.update(key(tea.KeyDown)) // free-text row
	f.update(runes("two"))
	f.update(key(tea.KeySpace))
	f.update(runes("words"))
	f.update(key(tea.KeyEnter))
	if len(f.answers) != 1 || f.answers[0].Value != "two words" {
		t.Fatalf("expected 'two words', got %+v", f.answers)
	}
}

func TestAskFormDigitSelectsOptionWhenNotEditing(t *testing.T) {
	// On an option row, a digit selects the matching option.
	f := formFor(singleQ()) // Alpha=1, Beta=2, Type something=3
	cmd := f.update(runes("2"))
	if !f.done {
		t.Fatal("digit should select option 2 and (single question) submit")
	}
	if f.answers[0].Value != "Beta" {
		t.Fatalf("expected Beta, got %+v", f.answers)
	}
	if _, ok := cmd().(askDoneMsg); !ok {
		t.Fatal("expected askDoneMsg")
	}
}

func TestAskFormDigitFocusesFreeTextRow(t *testing.T) {
	// Pressing the free-text row's number focuses it (does not submit/select).
	f := formFor(singleQ()) // free-text row is number 3
	f.update(runes("3"))
	if f.done {
		t.Fatal("focusing the free-text row must not submit")
	}
	if !f.onOtherRow() {
		t.Fatalf("expected cursor on free-text row, got %d", f.cursor)
	}
}

func TestAskFormDigitsTypedIntoFreeTextRow(t *testing.T) {
	// Once on the free-text row, digits are typed into it, not used to select.
	f := formFor(singleQ())
	f.update(key(tea.KeyDown))
	f.update(key(tea.KeyDown)) // free-text row
	f.update(runes("1"))
	f.update(runes("0"))
	if f.done {
		t.Fatal("typing digits into the free-text row must not select an option")
	}
	if got := string(f.otherText[f.tab]); got != "10" {
		t.Fatalf("digits not typed into free-text row: %q", got)
	}
	f.update(key(tea.KeyEnter))
	if f.answers[0].Value != "10" {
		t.Fatalf("expected free-text answer '10', got %+v", f.answers)
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
	f.update(key(tea.KeyEnter)) // answer q1 -> 1a, move to q2 tab
	if f.done {
		t.Fatal("should not be done after first question")
	}
	f.update(key(tea.KeyEnter)) // answer q2 -> 2a, land on Submit tab
	if f.done {
		t.Fatal("answering the last question should land on Submit, not finish")
	}
	if !f.onSubmit() {
		t.Fatalf("expected to be on Submit tab, tab=%d", f.tab)
	}
	cmd := f.update(key(tea.KeyEnter)) // Submit answers (cursor 0)
	if !f.done || f.cancelled {
		t.Fatalf("expected submitted (done && !cancelled)")
	}
	if len(f.answers) != 2 || f.answers[0].QuestionText != "First?" || f.answers[1].QuestionText != "Second?" {
		t.Fatalf("answers out of order: %+v", f.answers)
	}
	if f.answers[0].Value != "1a" || f.answers[1].Value != "2a" {
		t.Fatalf("unexpected values: %+v", f.answers)
	}
	if _, ok := cmd().(askDoneMsg); !ok {
		t.Fatal("expected askDoneMsg")
	}
}

func TestAskFormSubmitTabCancel(t *testing.T) {
	q1 := tools.Question{Prompt: "First?", Options: []tools.Option{{Label: "1a"}, {Label: "1b"}}}
	q2 := tools.Question{Prompt: "Second?", Options: []tools.Option{{Label: "2a"}, {Label: "2b"}}}
	f := formFor(q1, q2)
	f.update(key(tea.KeyEnter)) // q1
	f.update(key(tea.KeyEnter)) // q2 -> Submit tab
	f.update(key(tea.KeyDown))  // move to "Cancel"
	f.update(key(tea.KeyEnter)) // choose Cancel
	if !f.cancelled || !f.done {
		t.Fatalf("expected cancelled via Submit-tab Cancel option")
	}
}

func TestAskFormViewRendersOptionsAndDescriptions(t *testing.T) {
	f := formFor(singleQ())
	out := f.view(80)
	// Every option's description must be shown (not only the focused one).
	for _, want := range []string{"Pick one?", "Alpha", "Beta", "first", "second", otherPlaceholder} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q:\n%s", want, out)
		}
	}
}

func TestAskFormViewTabBarForMultipleQuestions(t *testing.T) {
	q1 := tools.Question{Header: "First", Prompt: "Q1?", Options: []tools.Option{{Label: "a"}, {Label: "b"}}}
	q2 := tools.Question{Header: "Second", Prompt: "Q2?", Options: []tools.Option{{Label: "c"}, {Label: "d"}}}
	out := formFor(q1, q2).view(80)
	for _, want := range []string{"First", "Second", "Submit", "☐"} {
		if !strings.Contains(out, want) {
			t.Fatalf("tab bar missing %q:\n%s", want, out)
		}
	}
}

func TestAskFormPreviewOnMultiSelectHarmless(t *testing.T) {
	q := tools.Question{
		Prompt:      "Pick many?",
		MultiSelect: true,
		Options:     []tools.Option{{Label: "a", Preview: "PREV-A"}, {Label: "b"}},
	}
	f := formFor(q)
	out := f.view(80) // must not panic; renders numbered checkboxes
	if !strings.Contains(out, "[ ] 1. a") || !strings.Contains(out, "[ ] 2. b") {
		t.Fatalf("multiSelect checkboxes missing: %q", out)
	}
}
