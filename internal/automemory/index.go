package automemory

import (
	"fmt"
	"strings"
)

// Index bounds, matching Claude Code's limits (CON-005 / REQ-007).
const (
	// maxIndexLines caps how many memory entries appear in a scope's index.
	maxIndexLines = 200
	// maxIndexBytes caps the rendered index size (~25 KB).
	maxIndexBytes = 25000
	// maxIndexLineLen caps each index line length in characters (< 150).
	maxIndexLineLen = 149
)

// BuildIndex regenerates a scope's MEMORY.md index from the memory files on disk
// (PLD-9). The result is the injection source of truth and can never drift from
// the files. Entries are one line each, capped at maxIndexLineLen characters; the
// whole index is truncated at maxIndexLines entries and maxIndexBytes with a
// visible notice when a scope exceeds the bounds. An empty scope yields an empty
// string.
func (s *Store) BuildIndex(scope Scope) (string, error) {
	memories, err := s.Load(scope)
	if err != nil {
		return "", err
	}
	if len(memories) == 0 {
		return "", nil
	}

	var lines []string
	byteCount := 0
	shown := 0
	for _, m := range memories {
		line := renderIndexEntry(m)
		if shown >= maxIndexLines || byteCount+len(line)+1 > maxIndexBytes {
			break
		}
		lines = append(lines, line)
		byteCount += len(line) + 1
		shown++
	}

	if shown < len(memories) {
		lines = append(lines, fmt.Sprintf(
			"- … (%d more memories not shown; index truncated at %d entries / %d bytes — prune or consolidate)",
			len(memories)-shown, maxIndexLines, maxIndexBytes,
		))
	}
	return strings.Join(lines, "\n"), nil
}

// renderIndexEntry formats one memory as a single-line index pointer of the form
// "- [name](name.md) — description", collapsed to one line and truncated to the
// per-line character cap.
func renderIndexEntry(m Memory) string {
	desc := strings.Join(strings.Fields(m.Description), " ")
	line := fmt.Sprintf("- [%s](%s.md) — %s", m.Name, m.Name, desc)
	return truncateRunes(line, maxIndexLineLen)
}

// truncateRunes shortens s to at most max characters, appending an ellipsis when
// truncation occurs.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max <= 1 {
		return string(r[:max])
	}
	return string(r[:max-1]) + "…"
}
