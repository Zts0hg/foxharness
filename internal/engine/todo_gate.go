package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/session"
)

func (e *AgentEngine) todoCompletionReminder(sess *session.Session) string {
	if sess == nil || !e.hasTool("update_todo") {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(sess.RootDir, "TODO.md"))
	if err != nil {
		return ""
	}
	items := incompleteTodoItems(string(data))
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("TODO.md still has incomplete checklist items. Before giving the final answer, call update_todo with the complete TODO.md content and mark completed items as [x]. If an item is genuinely not complete, keep it unchecked and explain why in the final answer.\n\nIncomplete items:")
	for _, item := range items {
		b.WriteString("\n- ")
		b.WriteString(item)
	}
	return b.String()
}

func (e *AgentEngine) hasTool(name string) bool {
	for _, def := range e.registry.GetAvailableTools() {
		if def.Name == name {
			return true
		}
	}
	return false
}

func incompleteTodoItems(content string) []string {
	var items []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [ ]") {
			continue
		}
		item := strings.TrimSpace(strings.TrimPrefix(trimmed, "- [ ]"))
		if item == "" || strings.EqualFold(strings.Trim(item, "."), "not recorded") {
			continue
		}
		items = append(items, item)
	}
	return items
}

func todoCompletionReminderMessage(reminder string) string {
	return fmt.Sprintf("[Runtime System Reminder]\n\n%s", reminder)
}
