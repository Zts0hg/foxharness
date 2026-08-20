package runtimecompaction

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
)

func TestAdapterProposesDurableInitialHistoryCompaction(t *testing.T) {
	mechanism := &stubMechanism{estimate: 2, threshold: 1, recentKeep: 1, summary: "summary"}
	adapter := New(mechanism)
	records := []session.MessageRecord{
		{Seq: 0, Message: schema.Message{Role: schema.RoleUser, Content: "old user"}},
		{Seq: 1, Message: schema.Message{Role: schema.RoleAssistant, Content: "old answer"}},
		{Seq: 2, Message: schema.Message{Role: schema.RoleUser, Content: "current"}},
	}
	proposal, err := adapter.Compact(context.Background(), foxruntime.ContextCompactionRequest{
		Trigger:  foxruntime.ContextCompactionInitialHistory,
		Messages: []engine.Message{{Role: engine.RoleSystem, Content: "system"}, {Role: engine.RoleUser, Content: "old user"}, {Role: engine.RoleAssistant, Content: "old answer"}, {Role: engine.RoleUser, Content: "current"}},
		Records:  records, TranscriptPath: "/session/transcript.jsonl",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.Changed || proposal.CompactState == nil || proposal.CompactState.Summary != "summary" || proposal.CompactState.CoveredUntilSeq != 0 || proposal.Messages != nil {
		t.Fatalf("initial proposal = %#v", proposal)
	}
	wantSummaryInput := []schema.Message{{Role: schema.RoleUser, Content: "old user"}}
	if !reflect.DeepEqual(mechanism.summarized, wantSummaryInput) {
		t.Fatalf("summarized = %#v, want %#v", mechanism.summarized, wantSummaryInput)
	}
}

func TestAdapterUsesRunLocalAutomaticAndReactiveCompaction(t *testing.T) {
	original := []engine.Message{{Role: engine.RoleSystem, Content: "system"}, {Role: engine.RoleUser, Content: "old"}, {Role: engine.RoleUser, Content: "current"}}
	compacted := []engine.Message{{Role: engine.RoleSystem, Content: "system"}, {Role: engine.RoleUser, Content: "summary"}}
	mechanism := &stubMechanism{maybe: compacted, force: compacted}
	adapter := New(mechanism)
	definitions := []engine.ToolDefinition{{Name: "read_file"}}

	automatic, err := adapter.Compact(context.Background(), foxruntime.ContextCompactionRequest{
		Trigger: foxruntime.ContextCompactionPreTurn, Messages: original, ToolDefinitions: definitions,
	})
	if err != nil {
		t.Fatal(err)
	}
	reactive, err := adapter.Compact(context.Background(), foxruntime.ContextCompactionRequest{
		Trigger: foxruntime.ContextCompactionReactive, Messages: original, ToolDefinitions: definitions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !automatic.Changed || automatic.CompactState != nil || !reflect.DeepEqual(automatic.Messages, compacted) {
		t.Fatalf("automatic proposal = %#v", automatic)
	}
	if !reactive.Changed || reactive.CompactState != nil || !reflect.DeepEqual(reactive.Messages, compacted) {
		t.Fatalf("reactive proposal = %#v", reactive)
	}
	if mechanism.toolOverhead <= 0 || mechanism.maybeCalls != 1 || mechanism.forceCalls != 1 {
		t.Fatalf("mechanism calls = overhead:%d maybe:%d force:%d", mechanism.toolOverhead, mechanism.maybeCalls, mechanism.forceCalls)
	}
}

func TestAdapterManualCompactionCoversPersistedRecords(t *testing.T) {
	mechanism := &stubMechanism{summary: "manual summary"}
	adapter := New(mechanism)
	proposal, err := adapter.Compact(context.Background(), foxruntime.ContextCompactionRequest{
		Trigger:      foxruntime.ContextCompactionManual,
		Messages:     []engine.Message{{Role: engine.RoleUser, Content: "history"}},
		Records:      []session.MessageRecord{{Seq: 4, Message: schema.Message{Role: schema.RoleUser, Content: "history"}}},
		Instructions: "retain decisions",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !proposal.Changed || proposal.CompactState == nil || proposal.CompactState.CoveredUntilSeq != 4 || mechanism.instructions != "retain decisions" {
		t.Fatalf("manual proposal = %#v, instructions = %q", proposal, mechanism.instructions)
	}
}

func TestAdapterChecksBlockingBudgetAfterToolOverhead(t *testing.T) {
	mechanism := &stubMechanism{estimate: 9, blocking: 9}
	adapter := New(mechanism)
	err := adapter.CheckContext(context.Background(), foxruntime.ContextBudgetRequest{
		Messages:        []engine.Message{{Role: engine.RoleUser, Content: "request"}},
		ToolDefinitions: []engine.ToolDefinition{{Name: "read_file"}},
	})
	var blocked *foxruntime.ContextBlockedError
	if !errors.As(err, &blocked) || blocked.UsedTokens != 9 || blocked.Limit >= 9 || mechanism.toolOverhead <= 0 {
		t.Fatalf("budget error = %#v, overhead = %d", err, mechanism.toolOverhead)
	}
}

func TestAdapterRejectsMissingMechanismWithoutPanic(t *testing.T) {
	adapter := New(nil)
	if _, err := adapter.Compact(context.Background(), foxruntime.ContextCompactionRequest{Trigger: foxruntime.ContextCompactionPreTurn}); err == nil {
		t.Fatal("Compact() error = nil")
	}
	if err := adapter.CheckContext(context.Background(), foxruntime.ContextBudgetRequest{}); err == nil {
		t.Fatal("CheckContext() error = nil")
	}
}

type stubMechanism struct {
	estimate     int
	threshold    int
	recentKeep   int
	blocking     int
	toolOverhead int
	summary      string
	summarized   []schema.Message
	instructions string
	maybe        []schema.Message
	force        []schema.Message
	maybeCalls   int
	forceCalls   int
}

func (m *stubMechanism) Estimate([]schema.Message) int { return m.estimate }
func (m *stubMechanism) SetToolOverhead(value int)     { m.toolOverhead = value }
func (m *stubMechanism) Threshold() int                { return m.threshold }
func (m *stubMechanism) RecentKeep() int               { return m.recentKeep }
func (m *stubMechanism) BlockingThreshold() int        { return m.blocking - m.toolOverhead }
func (m *stubMechanism) Summarize(_ context.Context, messages []schema.Message) (string, error) {
	m.summarized = append([]schema.Message(nil), messages...)
	return m.summary, nil
}
func (m *stubMechanism) SummarizeWithInstructions(_ context.Context, messages []schema.Message, instructions string) (string, error) {
	m.summarized = append([]schema.Message(nil), messages...)
	m.instructions = instructions
	return m.summary, nil
}
func (m *stubMechanism) MaybeCompact(context.Context, []schema.Message) ([]schema.Message, error) {
	m.maybeCalls++
	return append([]schema.Message(nil), m.maybe...), nil
}
func (m *stubMechanism) ForceCompact(context.Context, []schema.Message) ([]schema.Message, error) {
	m.forceCalls++
	return append([]schema.Message(nil), m.force...), nil
}
