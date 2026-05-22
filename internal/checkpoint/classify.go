package checkpoint

import (
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

// IsSynthetic returns true for non-meaningful messages such as progress,
// system, empty tool results, meta messages, compact summaries, and
// transcript-only entries.
func IsSynthetic(rec session.MessageRecord) bool {
	if rec.IsMeta || rec.IsCompactSummary || rec.IsVisibleInTranscriptOnly {
		return true
	}
	kind := strings.ToLower(strings.TrimSpace(rec.Kind))
	if kind == "progress" || kind == "system" {
		return true
	}
	msg := rec.Message
	if msg.Role == schema.RoleSystem {
		return true
	}
	if msg.ToolCallID != "" && strings.TrimSpace(msg.Content) == "" {
		return true
	}
	return isCompactionSummaryContent(msg.Content)
}

// IsMeaningful returns true if a message contains real assistant content or a
// non-empty tool result.
func IsMeaningful(rec session.MessageRecord) bool {
	msg := rec.Message
	if msg.Role == schema.RoleAssistant {
		return strings.TrimSpace(msg.Content) != "" || len(msg.ToolCalls) > 0
	}
	return msg.ToolCallID != "" && strings.TrimSpace(msg.Content) != ""
}

// SelectableMessages filters message records to human-authored user prompts.
func SelectableMessages(records []session.MessageRecord) []SelectableMessage {
	selectable := make([]SelectableMessage, 0, len(records))
	for _, rec := range records {
		msg := rec.Message
		if msg.Role != schema.RoleUser || msg.ToolCallID != "" {
			continue
		}
		if rec.IsMeta || rec.IsCompactSummary || rec.IsVisibleInTranscriptOnly {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || isCompactionSummaryContent(content) {
			continue
		}
		selectable = append(selectable, SelectableMessage{
			Seq:       rec.Seq,
			Content:   content,
			Timestamp: rec.Time,
		})
	}
	return selectable
}

// MessagesAfterAreOnlySynthetic reports whether all records after index are
// synthetic.
func MessagesAfterAreOnlySynthetic(records []session.MessageRecord, index int) bool {
	for i := index + 1; i < len(records); i++ {
		if !IsSynthetic(records[i]) {
			return false
		}
	}
	return true
}

func isCompactionSummaryContent(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "## Compacted Context Summary")
}
