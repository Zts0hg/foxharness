package selector

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	selectorTitleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffe3a8"))
	selectorCursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8855"))
	selectorMutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#d69d60"))
	selectorSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffc56b"))
)

// Palette contains the theme colors used by the selector overlay.
type Palette struct {
	Title    lipgloss.Color
	Cursor   lipgloss.Color
	Muted    lipgloss.Color
	Selected lipgloss.Color
}

// ApplyPalette rebuilds selector styles from the active TUI theme.
func ApplyPalette(p Palette) {
	selectorTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(p.Title)
	selectorCursorStyle = lipgloss.NewStyle().Foreground(p.Cursor)
	selectorMutedStyle = lipgloss.NewStyle().Foreground(p.Muted)
	selectorSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(p.Selected)
}

// View renders the selector.
func (m Model) View() string {
	switch m.state {
	case previewView:
		return m.previewView()
	default:
		return m.listView()
	}
}

func (m Model) listView() string {
	lines := []string{
		selectorTitleStyle.Render("Rewind"),
		"",
		selectorMutedStyle.Render("Restore the code and/or conversation to the point before..."),
		"",
	}
	for i, msg := range m.messages {
		cursor := "  "
		if i == m.cursor {
			cursor = selectorCursorStyle.Render("❯ ")
		}
		if msg.IsCurrent {
			lines = append(lines, fmt.Sprintf("%s%s", cursor, selectorSelectedStyle.Render("(current)")))
			continue
		}
		content := ansi.Truncate(strings.ReplaceAll(msg.Content, "\n", " "), 96, "...")
		lines = append(lines,
			fmt.Sprintf("%s%s", cursor, selectorSelectedStyle.Render(content)),
			"  "+selectorMutedStyle.Render(m.listSummary(msg)),
			"",
		)
	}
	lines = append(lines, "", selectorMutedStyle.Render("Enter to continue · Esc/Q to exit"))
	return strings.Join(lines, "\n")
}

func (m Model) listSummary(msg checkpoint.SelectableMessage) string {
	if err := m.listErrors[msg.Seq]; err != nil {
		return "Diff unavailable: " + err.Error()
	}
	stats := m.statsFor(msg)
	if stats.FilesChanged == 0 {
		return "No code changes"
	}
	if stats.FilesChanged == 1 {
		file := "1 file changed"
		if len(stats.ChangedFiles) > 0 {
			file = filepath.Base(stats.ChangedFiles[0])
		}
		return fmt.Sprintf("%s +%d -%d", file, stats.Insertions, stats.Deletions)
	}
	return fmt.Sprintf("%d files changed +%d -%d", stats.FilesChanged, stats.Insertions, stats.Deletions)
}

func (m Model) previewView() string {
	stats := m.diffStats
	if stats == nil {
		stats = &checkpoint.DiffStats{}
	}
	lines := []string{
		selectorTitleStyle.Render("Checkpoint preview"),
		fmt.Sprintf("%d files changed, +%d -%d", stats.FilesChanged, stats.Insertions, stats.Deletions),
	}
	if m.err != nil {
		lines = append(lines, selectorMutedStyle.Render(m.err.Error()))
	}
	for _, file := range stats.ChangedFiles {
		lines = append(lines, "  "+file)
	}
	lines = append(lines, "")
	options := []string{
		"Restore code and conversation",
		"Restore conversation only",
		"Restore code only",
		"Cancel",
	}
	for i, option := range options {
		prefix := "  "
		if i == m.optionCursor {
			prefix = selectorCursorStyle.Render("❯ ")
		}
		lines = append(lines, fmt.Sprintf("%s%d. %s", prefix, i+1, option))
	}
	lines = append(lines, selectorMutedStyle.Render("Enter to choose · Esc back · Q to exit"))
	return strings.Join(lines, "\n")
}
