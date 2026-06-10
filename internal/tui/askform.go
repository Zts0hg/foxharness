package tui

import (
	"strconv"
	"strings"

	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// askFocusedStyle highlights the option row under the cursor.
var askFocusedStyle = lipgloss.NewStyle().Foreground(cAccentHi)

// askDoneMsg signals that the active askForm finished (answered or cancelled).
// The model reads the form's accumulated state and replies on the request's
// channel, then clears the overlay.
type askDoneMsg struct{}

// otherOptionLabel is the synthetic free-text choice appended to every question
// (REQ-008). The LLM must not supply it itself.
const otherOptionLabel = "Other (type your own)"

// askForm drives one askRequest across its questions inside the TUI: it presents
// a selectable list per question, supports multi-select toggling and an auto
// "Other" free-text entry, and accumulates one tools.Answer per question.
type askForm struct {
	req      askRequest
	qIndex   int
	cursor   int
	selected map[int]bool
	answers  []tools.Answer

	otherMode bool
	otherText []rune

	done      bool
	cancelled bool
}

// newAskForm creates an overlay for the given request, positioned at its first
// question.
func newAskForm(req askRequest) *askForm {
	return &askForm{
		req:      req,
		selected: make(map[int]bool),
		answers:  make([]tools.Answer, 0, len(req.questions)),
	}
}

// current returns the question currently being answered.
func (f *askForm) current() tools.Question {
	return f.req.questions[f.qIndex]
}

// optionCount is the number of selectable rows including the trailing "Other".
func (f *askForm) optionCount() int {
	return len(f.current().Options) + 1
}

// isOtherRow reports whether the cursor is on the synthetic "Other" row.
func (f *askForm) isOtherRow() bool {
	return f.cursor == len(f.current().Options)
}

// update applies a key event and returns a command emitting askDoneMsg once the
// form completes (answered all questions or cancelled).
func (f *askForm) update(msg tea.KeyMsg) tea.Cmd {
	if f.otherMode {
		return f.updateOther(msg)
	}

	switch msg.Type {
	case tea.KeyEsc:
		f.cancelled = true
		f.done = true
		return askDoneCmd()
	case tea.KeyUp:
		f.moveCursor(-1)
	case tea.KeyDown:
		f.moveCursor(1)
	case tea.KeySpace:
		if f.current().MultiSelect && !f.isOtherRow() {
			f.selected[f.cursor] = !f.selected[f.cursor]
		}
	case tea.KeyEnter:
		return f.confirm()
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "k":
			f.moveCursor(-1)
		case "j":
			f.moveCursor(1)
		case " ":
			if f.current().MultiSelect && !f.isOtherRow() {
				f.selected[f.cursor] = !f.selected[f.cursor]
			}
		}
	}
	return nil
}

// updateOther handles text entry while the user types an "Other" answer.
func (f *askForm) updateOther(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		// Cancel the free-text entry and return to the option list.
		f.otherMode = false
		f.otherText = nil
		return nil
	case tea.KeyEnter:
		f.recordAnswer(strings.TrimSpace(string(f.otherText)), "")
		f.otherMode = false
		f.otherText = nil
		return f.advance()
	case tea.KeyBackspace:
		if len(f.otherText) > 0 {
			f.otherText = f.otherText[:len(f.otherText)-1]
		}
	case tea.KeySpace:
		f.otherText = append(f.otherText, ' ')
	case tea.KeyRunes:
		f.otherText = append(f.otherText, msg.Runes...)
	}
	return nil
}

// confirm finalizes the current question based on the cursor/selection state.
func (f *askForm) confirm() tea.Cmd {
	q := f.current()
	if f.isOtherRow() {
		f.otherMode = true
		return nil
	}
	if q.MultiSelect {
		var labels []string
		for i := 0; i < len(q.Options); i++ {
			if f.selected[i] {
				labels = append(labels, q.Options[i].Label)
			}
		}
		if len(labels) == 0 {
			// Require at least one selection before confirming.
			return nil
		}
		f.recordAnswer(strings.Join(labels, ", "), "")
		return f.advance()
	}
	opt := q.Options[f.cursor]
	f.recordAnswer(opt.Label, opt.Preview)
	return f.advance()
}

// recordAnswer appends the answer for the current question.
func (f *askForm) recordAnswer(value, preview string) {
	f.answers = append(f.answers, tools.Answer{
		QuestionText: f.current().Prompt,
		Value:        value,
		Preview:      preview,
	})
}

// advance moves to the next question or finishes the form.
func (f *askForm) advance() tea.Cmd {
	f.qIndex++
	f.cursor = 0
	f.selected = make(map[int]bool)
	if f.qIndex >= len(f.req.questions) {
		f.done = true
		return askDoneCmd()
	}
	return nil
}

// moveCursor moves the highlighted row within bounds.
func (f *askForm) moveCursor(delta int) {
	f.cursor += delta
	if f.cursor < 0 {
		f.cursor = 0
	}
	if max := f.optionCount() - 1; f.cursor > max {
		f.cursor = max
	}
}

// view renders the overlay for the current question. It is shown inline at the
// bottom of the screen (in the input band), so it keeps to a compact card.
func (f *askForm) view(width int) string {
	if f.done {
		return ""
	}
	q := f.current()
	var b strings.Builder

	if head := f.headerLine(q); head != "" {
		b.WriteString(hintStyle.Render(head))
		b.WriteString("\n")
	}
	b.WriteString(headerStyle.Render(q.Prompt))
	b.WriteString("\n\n")

	for i, opt := range q.Options {
		b.WriteString(f.renderRow(i, opt.Label, q.MultiSelect))
		if i == f.cursor && opt.Description != "" {
			b.WriteString("\n" + mutedStyle.Render("      "+opt.Description))
		}
		b.WriteString("\n")
	}
	b.WriteString(f.renderRow(len(q.Options), otherOptionLabel, false))
	b.WriteString("\n")

	if f.otherMode {
		b.WriteString("\n  > " + string(f.otherText) + cursorStyle.Render("_") + "\n")
	} else if !f.isOtherRow() {
		if preview := q.Options[f.cursor].Preview; preview != "" {
			b.WriteString("\n" + mutedStyle.Render("preview:\n"+indent(preview)))
		}
	}

	hint := "[enter] select · [esc] cancel"
	if q.MultiSelect {
		hint = "[space] toggle · [enter] confirm · [esc] cancel"
	}
	b.WriteString("\n" + hintStyle.Render(hint))
	return b.String()
}

// headerLine builds the chip + progress line shown above the question.
func (f *askForm) headerLine(q tools.Question) string {
	var parts []string
	if q.Header != "" {
		parts = append(parts, "["+q.Header+"]")
	}
	if len(f.req.questions) > 1 {
		parts = append(parts, progress(f.qIndex+1, len(f.req.questions)))
	}
	return strings.Join(parts, " ")
}

// renderRow renders one selectable row with a cursor marker and, for
// multi-select questions, a checkbox. The focused row is highlighted.
func (f *askForm) renderRow(index int, label string, multi bool) string {
	focused := index == f.cursor
	marker := "  "
	if focused {
		marker = "> "
	}
	box := ""
	if multi {
		if f.selected[index] {
			box = "[x] "
		} else {
			box = "[ ] "
		}
	}
	row := marker + box + label
	if focused {
		return askFocusedStyle.Render(row)
	}
	return row
}

func progress(cur, total int) string {
	return "Question " + strconv.Itoa(cur) + "/" + strconv.Itoa(total)
}

func indent(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = "    " + ln
	}
	return strings.Join(lines, "\n")
}

func askDoneCmd() tea.Cmd {
	return func() tea.Msg { return askDoneMsg{} }
}
