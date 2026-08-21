package selector

import (
	"fmt"
	"strconv"

	"github.com/Zts0hg/foxharness/internal/app"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the rewind target selector and diff preview sub-model.
type Model struct {
	state        ViewState
	messages     []app.RewindTarget
	cursor       int
	optionCursor int
	diffStats    *app.RewindDiff
	listStats    map[int64]*app.RewindDiff
	listErrors   map[int64]string
	selected     app.RewindTarget
	err          string
}

// New creates a selector model with a virtual current-position entry.
func New(messages []app.RewindTarget) Model {
	copied := append([]app.RewindTarget(nil), messages...)
	copied = append(copied, app.RewindTarget{
		Sequence:  -1,
		Content:   "(current)",
		IsCurrent: true,
	})
	model := Model{
		state:      listView,
		messages:   copied,
		cursor:     len(copied) - 1,
		listStats:  make(map[int64]*app.RewindDiff),
		listErrors: make(map[int64]string),
	}
	model.loadListStats()
	return model
}

// Init returns the initial selector command.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles selector navigation and restore option choice.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	key := keyMsg.String()

	switch m.state {
	case listView:
		switch {
		case keyMatches(key, keys.up):
			m.moveCursor(-1)
			return m, nil
		case keyMatches(key, keys.down):
			m.moveCursor(1)
			return m, nil
		case keyMatches(key, keys.cancel):
			return m, resultCmd(ActionCancelled, "")
		case keyMatches(key, keys.selectK):
			if len(m.messages) == 0 {
				return m, resultCmd(ActionCancelled, "")
			}
			m.selected = m.messages[m.cursor]
			if m.selected.IsCurrent {
				return m, resultCmd(ActionNone, "")
			}
			m.state = previewView
			m.optionCursor = 0
			m.diffStats = m.statsFor(m.selected)
			m.err = m.listErrors[m.selected.Sequence]
			return m, nil
		}
	case previewView:
		switch {
		case keyMatches(key, keys.up):
			m.moveOption(-1)
			return m, nil
		case keyMatches(key, keys.down):
			m.moveOption(1)
			return m, nil
		case key == "esc":
			m.state = listView
			return m, nil
		case key == "q":
			return m, resultCmd(ActionCancelled, "")
		case keyMatches(key, keys.selectK):
			return m, resultCmd(m.selectedAction(), strconv.FormatInt(m.selected.Sequence, 10))
		case key == "1":
			return m, resultCmd(ActionRestoreBoth, strconv.FormatInt(m.selected.Sequence, 10))
		case key == "2":
			return m, resultCmd(ActionRestoreConversation, strconv.FormatInt(m.selected.Sequence, 10))
		case key == "3":
			return m, resultCmd(ActionRestoreCode, strconv.FormatInt(m.selected.Sequence, 10))
		case key == "4":
			return m, resultCmd(ActionCancelled, "")
		}
	}
	return m, nil
}

func (m *Model) loadListStats() {
	for _, msg := range m.messages {
		if msg.IsCurrent {
			continue
		}
		if msg.DiffError != "" {
			m.listErrors[msg.Sequence] = msg.DiffError
		}
		stats := msg.Diff
		stats.ChangedFiles = append([]string(nil), stats.ChangedFiles...)
		m.listStats[msg.Sequence] = &stats
	}
}

func (m Model) statsFor(msg app.RewindTarget) *app.RewindDiff {
	if stats := m.listStats[msg.Sequence]; stats != nil {
		return stats
	}
	return &app.RewindDiff{}
}

func (m *Model) moveCursor(delta int) {
	if len(m.messages) == 0 {
		m.cursor = 0
		return
	}
	m.cursor = (m.cursor + delta + len(m.messages)) % len(m.messages)
}

func (m *Model) moveOption(delta int) {
	const optionCount = 4
	m.optionCursor = (m.optionCursor + delta + optionCount) % optionCount
}

func (m Model) selectedAction() RestoreAction {
	switch m.optionCursor {
	case 0:
		return ActionRestoreBoth
	case 1:
		return ActionRestoreConversation
	case 2:
		return ActionRestoreCode
	default:
		return ActionCancelled
	}
}

func resultCmd(action RestoreAction, messageID string) tea.Cmd {
	return func() tea.Msg {
		return ResultMsg{Action: action, MessageID: messageID}
	}
}

func (a RestoreAction) String() string {
	switch a {
	case ActionRestoreBoth:
		return "restore both"
	case ActionRestoreConversation:
		return "restore conversation"
	case ActionRestoreCode:
		return "restore code"
	case ActionCancelled:
		return "cancelled"
	default:
		return fmt.Sprintf("action %d", int(a))
	}
}
