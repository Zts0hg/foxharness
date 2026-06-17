package tui

import (
	"strconv"
	"strings"

	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// askFocusedStyle highlights the active tab and the row under the cursor.
var askFocusedStyle = lipgloss.NewStyle().Foreground(cAccentHi)

// askDoneMsg signals that the active askForm finished (answered or cancelled).
// The model reads the form's accumulated state and replies on the request's
// channel, then clears the overlay.
type askDoneMsg struct{}

// otherPlaceholder is shown, placeholder-style, on the free-text row until the
// user types something (REQ-008's auto "Other" entry).
const otherPlaceholder = "Type something."

// descIndent aligns an option's description under its label (past "❯ N. ").
const descIndent = "     "

// askForm presents an askRequest as a tabbed multiple-choice form: each question
// is a tab (plus a trailing Submit tab when there is more than one), every option
// lists its description, and each question ends with an inline free-text row. The
// free-text row behaves like a focused text field: while the cursor is on it, all
// characters (including digits) are typed into it. State is kept per question so
// the user can move between tabs freely before submitting.
type askForm struct {
	req askRequest

	// tab is the active tab: 0..len(questions)-1 select a question; the value
	// len(questions) is the Submit tab (only reachable when there is >1 question).
	tab    int
	cursor int // row within the active tab (options + free-text, or Submit/Cancel)

	selected  []map[int]bool // per-question toggled real-option indices
	otherText [][]rune       // per-question free-text buffer

	answers   []tools.Answer // collected at submit time; read by the model
	done      bool
	cancelled bool
}

// newAskForm creates an overlay for the given request, positioned at its first
// question.
func newAskForm(req askRequest) *askForm {
	n := len(req.questions)
	f := &askForm{
		req:       req,
		selected:  make([]map[int]bool, n),
		otherText: make([][]rune, n),
	}
	for i := range f.selected {
		f.selected[i] = make(map[int]bool)
	}
	return f
}

func (f *askForm) submitTab() int { return len(f.req.questions) }
func (f *askForm) onSubmit() bool { return f.tab == f.submitTab() }
func (f *askForm) multiTab() bool { return len(f.req.questions) > 1 }

// current returns the question for the active tab. Only valid when !onSubmit.
func (f *askForm) current() tools.Question { return f.req.questions[f.tab] }

// otherRow is the cursor index of the free-text row for the active question.
func (f *askForm) otherRow() int { return len(f.current().Options) }

// onOtherRow reports whether the cursor is on the free-text row (and thus the
// field is focused for typing).
func (f *askForm) onOtherRow() bool {
	return !f.onSubmit() && f.cursor == f.otherRow()
}

// maxRow is the largest valid cursor index on the active tab.
func (f *askForm) maxRow() int {
	if f.onSubmit() {
		return 1 // 0 = Submit answers, 1 = Cancel
	}
	return f.otherRow() // options + free-text row
}

// answered reports whether question i has any selection or free-text answer.
func (f *askForm) answered(i int) bool {
	if len(f.selected[i]) > 0 {
		return true
	}
	return strings.TrimSpace(string(f.otherText[i])) != ""
}

// allAnswered reports whether every question has an answer.
func (f *askForm) allAnswered() bool {
	for i := range f.req.questions {
		if !f.answered(i) {
			return false
		}
	}
	return true
}

// update applies a key event and returns a command emitting askDoneMsg once the
// form is submitted or cancelled.
func (f *askForm) update(msg tea.KeyMsg) tea.Cmd {
	// When the cursor is on the free-text row it acts as a focused text field:
	// characters (including digits) are typed into it; only navigation keys move
	// away. This is what makes "Type something" directly editable.
	if f.onOtherRow() {
		return f.editOther(msg)
	}

	switch msg.Type {
	case tea.KeyEsc:
		return f.cancel()
	case tea.KeyLeft, tea.KeyShiftTab:
		f.gotoTab(f.tab - 1)
	case tea.KeyRight, tea.KeyTab:
		f.gotoTab(f.tab + 1)
	case tea.KeyUp:
		f.moveCursor(-1)
	case tea.KeyDown:
		f.moveCursor(1)
	case tea.KeySpace:
		f.toggleCurrent()
	case tea.KeyEnter:
		return f.enter()
	case tea.KeyRunes:
		return f.handleRunes(string(msg.Runes))
	}
	return nil
}

// handleRunes interprets character input on an option/Submit row (NOT the
// free-text row): digit keys pick the matching row, h/j/k/l mirror the arrows.
func (f *askForm) handleRunes(s string) tea.Cmd {
	if len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		n := int(s[0] - '1')
		if n > f.maxRow() {
			return nil
		}
		// Selecting the free-text row's number focuses it for typing rather than
		// committing a choice; the next keystroke goes into the field.
		if !f.onSubmit() && n == f.otherRow() {
			f.cursor = n
			return nil
		}
		f.cursor = n
		return f.enter()
	}
	switch s {
	case "h":
		f.gotoTab(f.tab - 1)
	case "l":
		f.gotoTab(f.tab + 1)
	case "k":
		f.moveCursor(-1)
	case "j":
		f.moveCursor(1)
	}
	return nil
}

// editOther handles input while the free-text row is focused.
func (f *askForm) editOther(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		return f.cancel()
	case tea.KeyLeft, tea.KeyShiftTab:
		f.gotoTab(f.tab - 1)
	case tea.KeyRight, tea.KeyTab:
		f.gotoTab(f.tab + 1)
	case tea.KeyUp:
		f.moveCursor(-1)
	case tea.KeyDown:
		f.moveCursor(1)
	case tea.KeyEnter:
		if strings.TrimSpace(string(f.otherText[f.tab])) == "" {
			return nil // nothing typed yet
		}
		return f.advanceTab()
	case tea.KeyBackspace, tea.KeyDelete:
		if buf := f.otherText[f.tab]; len(buf) > 0 {
			f.otherText[f.tab] = buf[:len(buf)-1]
		}
	case tea.KeySpace:
		f.otherText[f.tab] = append(f.otherText[f.tab], ' ')
	case tea.KeyRunes:
		f.otherText[f.tab] = append(f.otherText[f.tab], msg.Runes...)
	}
	return nil
}

// toggleCurrent flips the cursor option for a multi-select question.
func (f *askForm) toggleCurrent() {
	if f.onSubmit() || f.onOtherRow() || !f.current().MultiSelect {
		return
	}
	f.selected[f.tab][f.cursor] = !f.selected[f.tab][f.cursor]
}

// enter acts on the focused option/Submit row (never the free-text row, which is
// handled by editOther).
func (f *askForm) enter() tea.Cmd {
	if f.onSubmit() {
		if f.cursor == 1 {
			return f.cancel()
		}
		return f.submit()
	}
	q := f.current()
	if q.MultiSelect {
		if !f.answered(f.tab) {
			return nil // require at least one selection before advancing
		}
		return f.advanceTab()
	}
	// Single-select: this option becomes the sole answer.
	f.selected[f.tab] = map[int]bool{f.cursor: true}
	f.otherText[f.tab] = nil
	return f.advanceTab()
}

// advanceTab moves to the next tab, or submits immediately for a single-question
// form.
func (f *askForm) advanceTab() tea.Cmd {
	if !f.multiTab() {
		return f.submit()
	}
	f.gotoTab(f.tab + 1)
	return nil
}

// submit collects the answers and finishes the form.
func (f *askForm) submit() tea.Cmd {
	f.collect()
	f.cancelled = false
	f.done = true
	return askDoneCmd()
}

// cancel aborts the form.
func (f *askForm) cancel() tea.Cmd {
	f.cancelled = true
	f.done = true
	return askDoneCmd()
}

// collect builds the answer list from per-question state, in question order,
// omitting unanswered questions (partial answers are allowed, REQ-021).
func (f *askForm) collect() {
	f.answers = f.answers[:0]
	for i, q := range f.req.questions {
		if !f.answered(i) {
			continue
		}
		var labels []string
		preview := ""
		for j := 0; j < len(q.Options); j++ {
			if f.selected[i][j] {
				labels = append(labels, q.Options[j].Label)
				if !q.MultiSelect {
					preview = q.Options[j].Preview
				}
			}
		}
		if t := strings.TrimSpace(string(f.otherText[i])); t != "" {
			labels = append(labels, t)
			preview = ""
		}
		if len(labels) != 1 {
			preview = ""
		}
		f.answers = append(f.answers, tools.Answer{
			QuestionText: q.Prompt,
			Value:        strings.Join(labels, ", "),
			Preview:      preview,
		})
	}
}

// gotoTab clamps and switches the active tab, resetting the option cursor.
func (f *askForm) gotoTab(tab int) {
	if !f.multiTab() {
		return
	}
	if tab < 0 {
		tab = 0
	}
	if tab > f.submitTab() {
		tab = f.submitTab()
	}
	f.tab = tab
	f.cursor = 0
}

// moveCursor moves the highlighted row within bounds.
func (f *askForm) moveCursor(delta int) {
	f.cursor += delta
	if f.cursor < 0 {
		f.cursor = 0
	}
	if m := f.maxRow(); f.cursor > m {
		f.cursor = m
	}
}

// view renders the form inline at the bottom of the screen (the input band).
func (f *askForm) view(width int) string {
	if f.done {
		return ""
	}
	var b strings.Builder
	if f.multiTab() {
		b.WriteString(f.tabBar())
		b.WriteString("\n\n")
	}
	if f.onSubmit() {
		b.WriteString(f.submitView())
		return b.String()
	}

	q := f.current()
	b.WriteString(headerStyle.Render(q.Prompt))
	b.WriteString("\n\n")
	for i, opt := range q.Options {
		b.WriteString(f.renderRow(i, i+1, opt.Label, q.MultiSelect, f.selected[f.tab][i]))
		if opt.Description != "" {
			b.WriteString("\n" + mutedStyle.Render(descIndent+opt.Description))
		}
		b.WriteString("\n")
	}
	b.WriteString(f.renderOtherRow())
	b.WriteString("\n")

	hint := "Enter to select · digits to pick · ←/→ or Tab to switch · Esc to cancel"
	if q.MultiSelect {
		hint = "Space to toggle · Enter to confirm · ←/→ to switch · Esc to cancel"
	}
	if f.onOtherRow() {
		hint = "Type your answer · Enter to confirm · ↑ or ←/→ to leave · Esc to cancel"
	}
	b.WriteString("\n" + hintStyle.Render(hint))
	return b.String()
}

// renderOtherRow renders the inline free-text field with a placeholder until the
// user types, and a caret while focused.
func (f *askForm) renderOtherRow() string {
	idx := f.otherRow()
	focused := idx == f.cursor
	marker := "  "
	if focused {
		marker = "❯ "
	}
	prefix := marker + strconv.Itoa(idx+1) + ". "
	if focused {
		prefix = askFocusedStyle.Render(prefix)
	}

	text := string(f.otherText[f.tab])
	var body string
	if text == "" {
		body = placeholderStyle.Render(otherPlaceholder)
	} else {
		body = text
	}
	if focused {
		body += cursorStyle.Render("▏")
	}
	return prefix + body
}

// submitView renders the review-and-submit tab.
func (f *askForm) submitView() string {
	var b strings.Builder
	b.WriteString(headerStyle.Render("Review your answers"))
	b.WriteString("\n\n")
	for i, q := range f.req.questions {
		b.WriteString(" ● " + q.Prompt + "\n")
		if val := f.answerValue(i); val != "" {
			b.WriteString("   → " + val + "\n")
		} else {
			b.WriteString("   → " + mutedStyle.Render("(not answered)") + "\n")
		}
	}
	b.WriteString("\n")
	if f.allAnswered() {
		b.WriteString("Ready to submit your answers?")
	} else {
		b.WriteString(askFocusedStyle.Render("You have not answered all questions yet."))
	}
	b.WriteString("\n\n")
	b.WriteString(f.renderRow(0, 1, "Submit answers", false, false))
	b.WriteString("\n")
	b.WriteString(f.renderRow(1, 2, "Cancel", false, false))
	b.WriteString("\n\n" + hintStyle.Render("Enter to choose · ←/→ or Tab to switch · Esc to cancel"))
	return b.String()
}

// answerValue renders question i's current answer for the review list.
func (f *askForm) answerValue(i int) string {
	q := f.req.questions[i]
	var labels []string
	for j := 0; j < len(q.Options); j++ {
		if f.selected[i][j] {
			labels = append(labels, q.Options[j].Label)
		}
	}
	if t := strings.TrimSpace(string(f.otherText[i])); t != "" {
		labels = append(labels, t)
	}
	return strings.Join(labels, ", ")
}

// tabBar renders the question tabs plus the Submit tab.
func (f *askForm) tabBar() string {
	tabs := make([]string, 0, len(f.req.questions)+1)
	for i, q := range f.req.questions {
		mark := "☐"
		if f.answered(i) {
			mark = "☒"
		}
		tabs = append(tabs, f.styleTab(mark+" "+tabTitle(q, i), i == f.tab))
	}
	tabs = append(tabs, f.styleTab("✔ Submit", f.onSubmit()))
	return hintStyle.Render("← ") + strings.Join(tabs, "  ") + hintStyle.Render(" →")
}

func (f *askForm) styleTab(s string, active bool) string {
	if active {
		return askFocusedStyle.Render(s)
	}
	return hintStyle.Render(s)
}

func tabTitle(q tools.Question, i int) string {
	if q.Header != "" {
		return q.Header
	}
	return "Question " + strconv.Itoa(i+1)
}

// renderRow renders one numbered, selectable row, with a checkbox for
// multi-select options and a cursor marker when focused.
func (f *askForm) renderRow(idx, num int, label string, multi, sel bool) string {
	cursor := "  "
	if idx == f.cursor {
		cursor = "❯ "
	}
	box := ""
	if multi {
		if sel {
			box = "[x] "
		} else {
			box = "[ ] "
		}
	}
	row := cursor + box + strconv.Itoa(num) + ". " + label
	if idx == f.cursor {
		return askFocusedStyle.Render(row)
	}
	return row
}

func askDoneCmd() tea.Cmd {
	return func() tea.Msg { return askDoneMsg{} }
}
