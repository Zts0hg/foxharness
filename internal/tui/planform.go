package tui

import (
	"strings"

	"github.com/Zts0hg/foxharness/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
)

type planReviewDoneMsg struct{}

type planReviewForm struct {
	req planReviewRequest

	action   int
	feedback []rune
	scroll   int

	lineCount    int
	viewportRows int

	review    tools.PlanReview
	done      bool
	cancelled bool
}

func newPlanReviewForm(req planReviewRequest) *planReviewForm {
	return &planReviewForm{req: req}
}

func (f *planReviewForm) update(msg tea.KeyMsg) tea.Cmd {
	if f.done {
		return nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		return f.cancel()
	case tea.KeyTab, tea.KeyRight:
		f.action = (f.action + 1) % 2
	case tea.KeyShiftTab, tea.KeyLeft:
		f.action = (f.action + 1) % 2
	case tea.KeyUp:
		f.scrollBy(-1)
	case tea.KeyDown:
		f.scrollBy(1)
	case tea.KeyPgUp:
		f.scrollBy(-max(f.viewportRows-1, 1))
	case tea.KeyPgDown:
		f.scrollBy(max(f.viewportRows-1, 1))
	case tea.KeyHome:
		f.scroll = 0
	case tea.KeyEnd:
		f.scroll = f.maxScroll()
	case tea.KeyBackspace, tea.KeyDelete:
		if f.action == 1 && len(f.feedback) > 0 {
			f.feedback = f.feedback[:len(f.feedback)-1]
		}
	case tea.KeySpace:
		if f.action == 1 {
			f.feedback = append(f.feedback, ' ')
		}
	case tea.KeyRunes:
		if f.action == 1 {
			f.feedback = append(f.feedback, msg.Runes...)
		}
	case tea.KeyEnter:
		return f.submit()
	}
	return nil
}

func (f *planReviewForm) submit() tea.Cmd {
	f.done = true
	if f.action == 0 {
		f.review = tools.PlanReview{Decision: tools.PlanApproved}
	} else {
		f.review = tools.PlanReview{
			Decision: tools.PlanContinuePlanning,
			Feedback: string(f.feedback),
		}
	}
	return planReviewDoneCmd()
}

func (f *planReviewForm) cancel() tea.Cmd {
	f.done = true
	f.cancelled = true
	return planReviewDoneCmd()
}

func (f *planReviewForm) scrollBy(delta int) {
	f.scroll += delta
	if f.scroll < 0 {
		f.scroll = 0
	}
	if f.scroll > f.maxScroll() {
		f.scroll = f.maxScroll()
	}
}

func (f *planReviewForm) maxScroll() int {
	return max(f.lineCount-f.viewportRows, 0)
}

func (f *planReviewForm) view(width, maxHeight int) string {
	if f.done {
		return ""
	}
	contentWidth := max(width-inputStyle.GetHorizontalFrameSize()-2, 20)
	rendered := renderMarkdown(f.req.planMarkdown, contentWidth)
	lines := strings.Split(rendered, "\n")
	f.lineCount = len(lines)
	f.viewportRows = max(maxHeight-8, 3)
	if f.viewportRows > f.lineCount {
		f.viewportRows = f.lineCount
	}
	if f.scroll > f.maxScroll() {
		f.scroll = f.maxScroll()
	}
	end := min(f.scroll+f.viewportRows, len(lines))
	visible := lines[f.scroll:end]

	var b strings.Builder
	b.WriteString(headerStyle.Render("Review plan"))
	if f.lineCount > f.viewportRows {
		b.WriteString(hintStyle.Render("  PgUp/PgDown to scroll"))
	}
	b.WriteString("\n\n")
	b.WriteString(strings.Join(visible, "\n"))
	b.WriteString("\n\n")
	b.WriteString(f.renderAction(0, "Approve"))
	b.WriteString("\n")
	b.WriteString(f.renderAction(1, "Continue planning"))
	if f.action == 1 {
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render("Feedback (optional): "))
		if len(f.feedback) == 0 {
			b.WriteString(placeholderStyle.Render("Describe requested changes"))
		} else {
			b.WriteString(string(f.feedback))
		}
		b.WriteString(cursorStyle.Render("▏"))
	}
	b.WriteString("\n\n")
	b.WriteString(hintStyle.Render("Tab/←/→ choose · Enter confirm · Esc keep planning"))
	return b.String()
}

func (f *planReviewForm) renderAction(index int, label string) string {
	marker := "  "
	if f.action == index {
		marker = "❯ "
		return askFocusedStyle.Render(marker + label)
	}
	return marker + label
}

func planReviewDoneCmd() tea.Cmd {
	return func() tea.Msg { return planReviewDoneMsg{} }
}
