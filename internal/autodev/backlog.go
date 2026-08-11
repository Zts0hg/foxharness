package autodev

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxBacklogBytes int64 = 64 << 20

// itemHeading matches "## [type] Title" headings; the bracketed type is
// optional so plain "## Title" items still parse.
var itemHeading = regexp.MustCompile(`^##\s+(?:\[([^\]]+)\]\s*)?(.+?)\s*$`)

// fieldLine matches "**Field**: value" metadata lines under a heading.
var fieldLine = regexp.MustCompile(`^\*\*([A-Za-z]+)\*\*\s*:\s*(.*)$`)

// Parse reads the backlog markdown at path into ordered items (REQ-001).
// Each "## [type] Title" heading starts an item; "**ID**", "**Priority**",
// "**Status**", and "**Description**" lines fill its fields, with later
// plain lines appended to the description. A missing Status defaults to
// pending and a missing Priority defaults to the lowest bucket.
func Parse(path string) ([]Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open backlog: %w", err)
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil {
		return nil, fmt.Errorf("stat backlog: %w", err)
	} else if info.Size() > maxBacklogBytes {
		return nil, fmt.Errorf("backlog exceeds %d-byte limit", maxBacklogBytes)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBacklogBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read backlog: %w", err)
	}
	if int64(len(data)) > maxBacklogBytes {
		return nil, fmt.Errorf("backlog exceeds %d-byte limit", maxBacklogBytes)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("backlog is not valid UTF-8")
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	var items []Item
	var current *Item
	inDescription := false
	fence := ""

	flush := func() {
		if current == nil {
			return
		}
		current.Description = strings.Trim(current.Description, "\n")
		items = append(items, *current)
		current = nil
		fence = ""
	}

	for _, line := range strings.Split(content, "\n") {
		if current != nil && inDescription {
			if fence != "" {
				current.Description += "\n" + line
				if closesMarkdownFence(line, fence) {
					fence = ""
				}
				continue
			}
			if marker := markdownFence(line); marker != "" {
				current.Description += "\n" + line
				fence = marker
				continue
			}
		}

		if m := itemHeading.FindStringSubmatch(line); m != nil && strings.HasPrefix(line, "## ") {
			flush()
			current = &Item{
				Type:     strings.TrimSpace(m[1]),
				Title:    strings.TrimSpace(m[2]),
				Priority: PriorityLow,
				Status:   StatusPending,
			}
			inDescription = false
			continue
		}
		if current == nil {
			continue
		}
		if inDescription {
			current.Description += "\n" + line
			continue
		}

		if m := fieldLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			value := strings.TrimSpace(m[2])
			switch strings.ToLower(m[1]) {
			case "id":
				current.SourceID = value
				inDescription = false
			case "priority":
				current.Priority = parsePriority(value)
				inDescription = false
			case "status":
				current.Status = parseStatus(value)
				inDescription = false
			case "description":
				current.Description = value
				inDescription = true
			default:
				inDescription = false
			}
			continue
		}
	}
	flush()
	return items, nil
}

func markdownFence(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return ""
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == trimmed[0] {
		count++
	}
	if count < 3 {
		return ""
	}
	return trimmed[:count]
}

func closesMarkdownFence(line, fence string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, fence) {
		return false
	}
	for i := len(fence); i < len(trimmed); i++ {
		if trimmed[i] != fence[0] {
			return false
		}
	}
	return true
}

func parsePriority(s string) Priority {
	switch Priority(strings.ToLower(s)) {
	case PriorityHigh:
		return PriorityHigh
	case PriorityMedium:
		return PriorityMedium
	default:
		return PriorityLow
	}
}

func parseStatus(s string) Status {
	switch Status(strings.ToLower(s)) {
	case StatusInProgress:
		return StatusInProgress
	case StatusDone:
		return StatusDone
	default:
		return StatusPending
	}
}
