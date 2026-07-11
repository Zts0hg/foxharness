package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

var allStatuslineItems = []string{
	"model",
	"project",
	"git-branch",
	"run-state",
	"plan-mode",
	"context-used",
	"queued",
	"session-id",
	"theme",
	"sidebar",
}

var legacyDefaultStatuslineItems = []string{"model", "project", "git-branch", "context-used", "plan-mode"}

func statuslineAvailableItems() []string {
	items := append([]string(nil), allStatuslineItems...)
	sort.Strings(items)
	return items
}

func parseStatuslineItems(input string) ([]string, error) {
	rawItems := strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	if len(rawItems) == 0 {
		return nil, fmt.Errorf("No statusline items provided")
	}
	return normalizeStatuslineItems(rawItems, false)
}

func normalizeSavedStatuslineItems(items []string) []string {
	normalized, err := normalizeStatuslineItems(items, true)
	if err != nil || len(normalized) == 0 {
		return append([]string(nil), defaultStatuslineItems...)
	}
	if statuslineItemsEqual(normalized, legacyDefaultStatuslineItems) {
		return append([]string(nil), defaultStatuslineItems...)
	}
	return normalized
}

func normalizeStatuslineItems(items []string, ignoreUnknown bool) ([]string, error) {
	known := make(map[string]bool, len(allStatuslineItems))
	for _, item := range allStatuslineItems {
		known[item] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, raw := range items {
		item := strings.ToLower(strings.TrimSpace(raw))
		if item == "" {
			continue
		}
		if !known[item] {
			if ignoreUnknown {
				continue
			}
			return nil, fmt.Errorf("Unknown statusline item: %s", raw)
		}
		if seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out, nil
}

func (m Model) renderStatuslineItem(item string) string {
	switch item {
	case "model":
		return statusProjectStyle.Render(m.modelName)
	case "project":
		return statusProjectStyle.Render(m.project)
	case "git-branch":
		return mutedStyle.Render("git ") + statusProjectStyle.Render(m.gitBranch)
	case "run-state":
		return mutedStyle.Render("run ") + statusProjectStyle.Render(m.runStateLabel())
	case "plan-mode":
		return statusProjectStyle.Render("plan mode " + onOff(m.collaborationMode.PlanEnabled()))
	case "context-used":
		return mutedStyle.Render("Context ") + statusModelStyle.Render(normalizeContextUsage(m.contextUsage))
	case "queued":
		return mutedStyle.Render("queued ") + statusProjectStyle.Render(fmt.Sprintf("%d", len(m.queuedPrompts)))
	case "session-id":
		return mutedStyle.Render("sid ") + statusFaintStyle.Render(m.sessionID)
	case "theme":
		return mutedStyle.Render("theme ") + statusProjectStyle.Render(m.themeName)
	case "sidebar":
		return mutedStyle.Render("sidebar ") + statusProjectStyle.Render(m.sidebarStatusLabel())
	default:
		return ""
	}
}

func statuslineItemsText(items []string) string {
	if len(items) == 0 {
		return ""
	}
	return strings.Join(items, ", ")
}

func statuslineItemsEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
