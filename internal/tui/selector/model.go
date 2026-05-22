package selector

import (
	"fmt"
	"strconv"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	tea "github.com/charmbracelet/bubbletea"
)

// Model is the rewind target selector and diff preview sub-model.
type Model struct {
	state        ViewState
	messages     []checkpoint.SelectableMessage
	cursor       int
	optionCursor int
	diffStats    *checkpoint.DiffStats
	listStats    map[int64]*checkpoint.DiffStats
	listErrors   map[int64]error
	selected     checkpoint.SelectableMessage
	checkpointer checkpoint.Checkpointer
	err          error
}

// New creates a selector model with a virtual current-position entry.
func New(messages []checkpoint.SelectableMessage, cp checkpoint.Checkpointer) Model {
	copied := append([]checkpoint.SelectableMessage(nil), messages...)
	copied = append(copied, checkpoint.SelectableMessage{
		Seq:       -1,
		Content:   "(current)",
		IsCurrent: true,
	})
	model := Model{
		state:        listView,
		messages:     copied,
		cursor:       len(copied) - 1,
		listStats:    make(map[int64]*checkpoint.DiffStats),
		listErrors:   make(map[int64]error),
		checkpointer: cp,
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
			m.err = m.listErrors[m.selected.Seq]
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
			return m, resultCmd(m.selectedAction(), strconv.FormatInt(m.selected.Seq, 10))
		case key == "1":
			return m, resultCmd(ActionRestoreBoth, strconv.FormatInt(m.selected.Seq, 10))
		case key == "2":
			return m, resultCmd(ActionRestoreConversation, strconv.FormatInt(m.selected.Seq, 10))
		case key == "3":
			return m, resultCmd(ActionRestoreCode, strconv.FormatInt(m.selected.Seq, 10))
		case key == "4":
			return m, resultCmd(ActionCancelled, "")
		}
	}
	return m, nil
}

func (m *Model) loadListStats() {
	if m.checkpointer == nil {
		return
	}
	for _, msg := range m.messages {
		if msg.IsCurrent {
			continue
		}
		stats, err := m.checkpointer.GetDiffStats(strconv.FormatInt(msg.Seq, 10))
		if err != nil {
			m.listErrors[msg.Seq] = err
			continue
		}
		if stats == nil {
			stats = &checkpoint.DiffStats{}
		}
		m.listStats[msg.Seq] = stats
	}
}

func (m Model) statsFor(msg checkpoint.SelectableMessage) *checkpoint.DiffStats {
	if stats := m.listStats[msg.Seq]; stats != nil {
		return stats
	}
	return &checkpoint.DiffStats{}
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
