package engine

import (
	"context"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func (e *AgentEngine) buildInitialContext(
	ctx context.Context,
	sess *session.Session,
	systemPrompt string,
	history []session.MessageRecord,
	currentUser schema.Message,
) ([]schema.Message, bool, error) {
	systemMessage := schema.Message{
		Role:    schema.RoleSystem,
		Content: systemPrompt,
	}
	if e.compactor == nil {
		return rawContext(systemMessage, history, currentUser), false, nil
	}

	state, err := session.LoadCompactState(sess)
	if err != nil {
		return nil, false, err
	}

	transcriptPath := sess.TranscriptPath()
	projected := projectedContext(systemMessage, transcriptPath, state, history, currentUser)
	if e.compactor.Estimate(projected) < e.compactor.Threshold() {
		return projected, false, nil
	}

	active := recordsAfter(history, stateCoveredUntilSeq(state))
	split := len(active) - e.compactor.RecentKeep()
	if split <= 0 {
		return projected, false, nil
	}
	split = moveRecordSplitToProtocolBoundary(active, split)
	if split <= 0 {
		return projected, false, nil
	}

	toSummarize := make([]schema.Message, 0, split+1)
	if state.Summary != "" {
		toSummarize = append(toSummarize, compaction.BuildSummaryMessage(state.Summary, transcriptPath))
	}
	for _, record := range active[:split] {
		toSummarize = append(toSummarize, record.Message)
	}

	summary, err := e.compactor.Summarize(ctx, toSummarize)
	if err != nil {
		return projected, false, fmt.Errorf("持久化上下文压缩失败: %w", err)
	}

	nextState := &session.CompactState{
		Summary:         summary,
		CoveredUntilSeq: active[split-1].Seq,
	}
	if err := session.SaveCompactState(sess, nextState); err != nil {
		return nil, false, err
	}

	return projectedContext(systemMessage, transcriptPath, nextState, history, currentUser), true, nil
}

func rawContext(system schema.Message, history []session.MessageRecord, current schema.Message) []schema.Message {
	messages := make([]schema.Message, 0, len(history)+2)
	messages = append(messages, system)
	for _, record := range history {
		messages = append(messages, record.Message)
	}
	messages = append(messages, current)
	return messages
}

func projectedContext(system schema.Message, transcriptPath string, state *session.CompactState, history []session.MessageRecord, current schema.Message) []schema.Message {
	active := recordsAfter(history, stateCoveredUntilSeq(state))
	messages := make([]schema.Message, 0, len(active)+3)
	messages = append(messages, system)
	if state.Summary != "" {
		messages = append(messages, compaction.BuildSummaryMessage(state.Summary, transcriptPath))
	}
	for _, record := range active {
		messages = append(messages, record.Message)
	}
	messages = append(messages, current)
	return messages
}

func recordsAfter(records []session.MessageRecord, seq int64) []session.MessageRecord {
	if seq < 0 {
		return records
	}
	for i, record := range records {
		if record.Seq > seq {
			return records[i:]
		}
	}
	return nil
}

func stateCoveredUntilSeq(state *session.CompactState) int64 {
	if state == nil || state.Summary == "" {
		return -1
	}
	return state.CoveredUntilSeq
}

func moveRecordSplitToProtocolBoundary(records []session.MessageRecord, split int) int {
	if split >= len(records) {
		return split
	}
	for split > 0 && records[split].Message.ToolCallID != "" {
		split--
	}
	return split
}
