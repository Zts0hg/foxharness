/*
Package runtimecompaction adapts context compaction mechanics to runtime-owned
durable and run-local proposal contracts.
*/
package runtimecompaction

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/metrics"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

/* Mechanism is the compaction behavior required by the runtime adapter. */
type Mechanism interface {
	Estimate([]schema.Message) int
	SetToolOverhead(int)
	Threshold() int
	RecentKeep() int
	BlockingThreshold() int
	Summarize(context.Context, []schema.Message) (string, error)
	SummarizeWithInstructions(context.Context, []schema.Message, string) (string, error)
	MaybeCompact(context.Context, []schema.Message) ([]schema.Message, error)
	ForceCompact(context.Context, []schema.Message) ([]schema.Message, error)
}

/* Adapter translates runtime requests without committing persisted state. */
type Adapter struct {
	mechanism Mechanism
}

/* New constructs a runtime compaction adapter around one session-local mechanism. */
func New(mechanism Mechanism) *Adapter {
	return &Adapter{mechanism: mechanism}
}

/* Compact proposes one durable or run-local context reduction. */
func (a *Adapter) Compact(ctx context.Context, request foxruntime.ContextCompactionRequest) (foxruntime.ContextCompactionProposal, error) {
	if a == nil || isNilMechanism(a.mechanism) {
		return foxruntime.ContextCompactionProposal{}, errors.New("runtime compaction mechanism is required")
	}
	switch request.Trigger {
	case foxruntime.ContextCompactionInitialHistory:
		return a.compactInitial(ctx, request)
	case foxruntime.ContextCompactionPreTurn:
		return a.compactMessages(ctx, request, false)
	case foxruntime.ContextCompactionReactive:
		return a.compactMessages(ctx, request, true)
	case foxruntime.ContextCompactionManual:
		return a.compactManual(ctx, request)
	default:
		return foxruntime.ContextCompactionProposal{}, nil
	}
}

/* CheckContext rejects a projection at the mechanism's blocking threshold. */
func (a *Adapter) CheckContext(_ context.Context, request foxruntime.ContextBudgetRequest) error {
	if a == nil || isNilMechanism(a.mechanism) {
		return errors.New("runtime compaction mechanism is required")
	}
	a.mechanism.SetToolOverhead(metrics.EstimateToolDefinitions(metrics.RoughEstimator{}, request.ToolDefinitions))
	used := a.mechanism.Estimate(request.Messages)
	limit := a.mechanism.BlockingThreshold()
	if used >= limit {
		return &foxruntime.ContextBlockedError{UsedTokens: used, Limit: limit}
	}
	return nil
}

func isNilMechanism(value Mechanism) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}

func (a *Adapter) compactInitial(ctx context.Context, request foxruntime.ContextCompactionRequest) (foxruntime.ContextCompactionProposal, error) {
	if a.mechanism.Estimate(request.Messages) < a.mechanism.Threshold() {
		return foxruntime.ContextCompactionProposal{}, nil
	}
	history := request.Records
	if len(history) > 0 {
		history = history[:len(history)-1]
	}
	active := recordsAfter(history, coveredUntil(request.CompactState))
	split := len(active) - a.mechanism.RecentKeep()
	if split <= 0 {
		return foxruntime.ContextCompactionProposal{}, nil
	}
	split = moveRecordSplitToProtocolBoundary(active, split)
	if split <= 0 {
		return foxruntime.ContextCompactionProposal{}, nil
	}
	messages := make([]schema.Message, 0, split+1)
	if request.CompactState != nil && request.CompactState.Summary != "" {
		messages = append(messages, compaction.BuildSummaryMessage(request.CompactState.Summary, request.TranscriptPath))
	}
	for _, record := range active[:split] {
		messages = append(messages, record.Message)
	}
	summary, err := a.mechanism.Summarize(ctx, messages)
	if err != nil {
		return foxruntime.ContextCompactionProposal{}, fmt.Errorf("持久化上下文压缩失败: %w", err)
	}
	return foxruntime.ContextCompactionProposal{
		Changed:      true,
		CompactState: &session.CompactState{Summary: summary, CoveredUntilSeq: active[split-1].Seq},
	}, nil
}

func (a *Adapter) compactMessages(ctx context.Context, request foxruntime.ContextCompactionRequest, force bool) (foxruntime.ContextCompactionProposal, error) {
	a.mechanism.SetToolOverhead(metrics.EstimateToolDefinitions(metrics.RoughEstimator{}, request.ToolDefinitions))
	var messages []schema.Message
	var err error
	if force {
		messages, err = a.mechanism.ForceCompact(ctx, request.Messages)
	} else {
		messages, err = a.mechanism.MaybeCompact(ctx, request.Messages)
	}
	if err != nil {
		return foxruntime.ContextCompactionProposal{}, err
	}
	if reflect.DeepEqual(messages, request.Messages) {
		return foxruntime.ContextCompactionProposal{}, nil
	}
	return foxruntime.ContextCompactionProposal{Changed: true, Messages: cloneMessages(messages)}, nil
}

func (a *Adapter) compactManual(ctx context.Context, request foxruntime.ContextCompactionRequest) (foxruntime.ContextCompactionProposal, error) {
	if len(request.Records) == 0 {
		return foxruntime.ContextCompactionProposal{}, nil
	}
	summary, err := a.mechanism.SummarizeWithInstructions(ctx, request.Messages, request.Instructions)
	if err != nil {
		return foxruntime.ContextCompactionProposal{}, err
	}
	return foxruntime.ContextCompactionProposal{
		Changed:      true,
		CompactState: &session.CompactState{Summary: summary, CoveredUntilSeq: request.Records[len(request.Records)-1].Seq},
	}, nil
}

func recordsAfter(records []session.MessageRecord, seq int64) []session.MessageRecord {
	if seq < 0 {
		return records
	}
	for index, record := range records {
		if record.Seq > seq {
			return records[index:]
		}
	}
	return nil
}

func coveredUntil(state *session.CompactState) int64 {
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

func cloneMessages(messages []engine.Message) []engine.Message {
	result := make([]engine.Message, len(messages))
	for index, message := range messages {
		message.ToolCalls = append([]schema.ToolCall(nil), message.ToolCalls...)
		for callIndex := range message.ToolCalls {
			message.ToolCalls[callIndex].Arguments = append([]byte(nil), message.ToolCalls[callIndex].Arguments...)
		}
		result[index] = message
	}
	return result
}

var _ foxruntime.ContextCompactor = (*Adapter)(nil)
