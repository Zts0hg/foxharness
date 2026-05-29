package compaction

// CompactResult holds the outcome of a manual compaction for UI feedback.
type CompactResult struct {
	PreTokens          int
	PostTokens         int
	MessagesSummarized int
}
