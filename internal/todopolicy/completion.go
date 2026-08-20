package todopolicy

import (
	"os"
	"path/filepath"
	"strings"
)

/* CompletionReminder returns the current incomplete checklist gate when update_todo is available. */
func CompletionReminder(sessionRoot string, updateAvailable bool) string {
	if !updateAvailable {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(sessionRoot, "TODO.md"))
	if err != nil {
		return ""
	}
	items := incompleteItems(string(data))
	if len(items) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("TODO.md still has incomplete checklist items. Before giving the final answer, call update_todo with the complete TODO.md content and mark completed items as [x]. If an item is genuinely not complete, keep it unchecked and explain why in the final answer.\n\nIncomplete items:")
	for _, item := range items {
		builder.WriteString("\n- ")
		builder.WriteString(item)
	}
	return builder.String()
}

func incompleteItems(content string) []string {
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
