package compaction

import (
	"encoding/json"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// BoundaryMarkerPrefix is the human-visible prefix prepended to the JSON
// payload of a compaction boundary marker. It makes the system message easy
// to recognize when inspecting raw transcripts.
const BoundaryMarkerPrefix = "[compaction-boundary] "

// CompactBoundary describes a single compaction event. It is embedded in a
// system message to delimit the pre/post compaction message regions for
// future tooling (partial compaction, observability, etc.).
type CompactBoundary struct {
	Trigger            string `json:"trigger"`
	PreTokens          int    `json:"pre_tokens"`
	MessagesSummarized int    `json:"messages_summarized"`
	Timestamp          string `json:"timestamp"`
}

// BoundaryMessage renders a CompactBoundary as a system message whose content
// is a JSON document prefixed with BoundaryMarkerPrefix.
func BoundaryMessage(boundary CompactBoundary) schema.Message {
	data, _ := json.Marshal(boundary)
	return schema.Message{
		Role:    schema.RoleSystem,
		Content: BoundaryMarkerPrefix + string(data),
	}
}
