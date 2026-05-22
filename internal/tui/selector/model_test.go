package selector

import (
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	tea "github.com/charmbracelet/bubbletea"
)

type selectorCheckpointer struct {
	stats *checkpoint.DiffStats
}

func (c selectorCheckpointer) TrackEdit(filePath, messageID string) error { return nil }
func (c selectorCheckpointer) MakeSnapshot(messageID string) error        { return nil }
func (c selectorCheckpointer) Rewind(messageID string) ([]string, error)  { return nil, nil }
func (c selectorCheckpointer) GetDiffStats(messageID string) (*checkpoint.DiffStats, error) {
	return c.stats, nil
}
func (c selectorCheckpointer) HasAnyChanges(messageID string) (bool, error) { return false, nil }
func (c selectorCheckpointer) SetDisabled(disabled bool)                    {}
func (c selectorCheckpointer) IsDisabled() bool                             { return false }
func (c selectorCheckpointer) RestoreStateFromLog() error                   { return nil }

func TestSelectorStateTransition(t *testing.T) {
	m := New(selectorMessages(), selectorCheckpointer{})
	if m.cursor != len(m.messages)-1 || !m.messages[m.cursor].IsCurrent {
		t.Fatalf("initial cursor = %d, want current row at bottom", m.cursor)
	}
	next, cmd := m.Update(keyMsg("up"))
	if cmd != nil {
		t.Fatalf("up returned command")
	}
	m = next.(Model)
	next, cmd = m.Update(keyMsg("enter"))
	if cmd != nil {
		t.Fatalf("enter returned command in list preview transition")
	}
	m = next.(Model)
	if m.state != previewView {
		t.Fatalf("state = %v, want previewView", m.state)
	}

	next, cmd = m.Update(keyMsg("esc"))
	if cmd != nil {
		t.Fatalf("esc from preview returned command")
	}
	m = next.(Model)
	if m.state != listView {
		t.Fatalf("state = %v, want listView", m.state)
	}

	_, cmd = m.Update(keyMsg("q"))
	if cmd == nil {
		t.Fatalf("q from list did not return cancel command")
	}
	result := cmd()
	if got := result.(ResultMsg).Action; got != ActionCancelled {
		t.Fatalf("cancel action = %v, want ActionCancelled", got)
	}
}

func TestSelectorResultMsg(t *testing.T) {
	m := New(selectorMessages(), selectorCheckpointer{})
	next, _ := m.Update(keyMsg("up"))
	m = next.(Model)
	next, _ = m.Update(keyMsg("enter"))
	m = next.(Model)
	next, _ = m.Update(keyMsg("down"))
	m = next.(Model)
	_, cmd := m.Update(keyMsg("enter"))
	if cmd == nil {
		t.Fatalf("restore option enter did not return result command")
	}
	result := cmd().(ResultMsg)
	if result.Action != ActionRestoreConversation || result.MessageID != "7" {
		t.Fatalf("result = %#v, want conversation restore for 7", result)
	}
}

func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

func selectorMessages() []checkpoint.SelectableMessage {
	return []checkpoint.SelectableMessage{{
		Seq:       7,
		Content:   "change the parser",
		Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.Local),
	}}
}
