package autodev

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// itemHeading matches "## [type] Title" headings; the bracketed type is
// optional so plain "## Title" items still parse.
var itemHeading = regexp.MustCompile(`^##\s+(?:\[([^\]]+)\]\s*)?(.+?)\s*$`)

// fieldLine matches "**Field**: value" metadata lines under a heading.
var fieldLine = regexp.MustCompile(`^\*\*([A-Za-z]+)\*\*\s*:\s*(.*)$`)

// Parse reads the backlog markdown at path into ordered items (REQ-001).
// Each "## [type] Title" heading starts an item; "**Priority**",
// "**Status**", and "**Description**" lines fill its fields, with later
// plain lines appended to the description. A missing Status defaults to
// pending and a missing Priority defaults to the lowest bucket.
func Parse(path string) ([]Item, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open backlog: %w", err)
	}
	defer f.Close()

	var items []Item
	var current *Item
	inDescription := false

	flush := func() {
		if current == nil {
			return
		}
		current.Description = strings.TrimSpace(current.Description)
		items = append(items, *current)
		current = nil
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

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

		if m := fieldLine.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			value := strings.TrimSpace(m[2])
			switch strings.ToLower(m[1]) {
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

		if inDescription {
			current.Description += "\n" + line
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read backlog: %w", err)
	}
	flush()
	return items, nil
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
