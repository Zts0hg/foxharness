package app

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestInteractiveStateContractsKeepCommandsAndResultsApplicationOwned(t *testing.T) {
	state := InteractiveSessionState{
		Session:           SessionInfo{ID: "session-1", Directory: "/sessions/1"},
		WorkDir:           "/workspace",
		Model:             "fixture-model",
		Effort:            "high",
		ContextUsage:      "7%",
		CollaborationMode: "formal_plan",
		AutoMemoryIndex:   "- durable memory",
		RewindAvailable:   true,
	}
	record := ConversationRecord{
		Sequence: 7,
		Time:     time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC),
		Role:     "assistant",
		Content:  "done",
		ToolCalls: []ConversationToolCall{{
			ID: "call-1", Name: "read_file", Arguments: `{"path":"main.go"}`,
		}},
	}
	target := RewindTarget{
		Sequence: 5, Content: "restore here",
		Diff: RewindDiff{FilesChanged: 1, Insertions: 2, ChangedFiles: []string{"main.go"}},
	}

	reader := &contractStateReader{state: state, records: []ConversationRecord{record}, targets: []RewindTarget{target}}
	gotState := reader.State()
	gotRecords, _ := reader.Conversation(context.Background())
	gotTargets, _ := reader.RewindTargets(context.Background())
	if !reflect.DeepEqual(gotState, state) || !reflect.DeepEqual(gotRecords, []ConversationRecord{record}) || !reflect.DeepEqual(gotTargets, []RewindTarget{target}) {
		t.Fatalf("application state contracts changed values: state=%#v records=%#v targets=%#v", gotState, gotRecords, gotTargets)
	}

	var _ InteractiveStateReader = reader
	var _ InteractiveStateController = (*contractStateController)(nil)
}

type contractStateReader struct {
	state   InteractiveSessionState
	records []ConversationRecord
	targets []RewindTarget
}

func (r *contractStateReader) State() InteractiveSessionState { return r.state }
func (r *contractStateReader) Conversation(context.Context) ([]ConversationRecord, error) {
	return r.records, nil
}
func (*contractStateReader) ProjectInputHistory(context.Context, int) ([]string, error) {
	return nil, nil
}
func (r *contractStateReader) RewindTargets(context.Context) ([]RewindTarget, error) {
	return r.targets, nil
}

type contractStateController struct{}

func (*contractStateController) NewSession(context.Context, NewSessionCommand) (InteractiveSessionState, error) {
	return InteractiveSessionState{}, nil
}
func (*contractStateController) UpdateModel(context.Context, ModelCommand) (InteractiveSessionState, error) {
	return InteractiveSessionState{}, nil
}
func (*contractStateController) UpdateEffort(context.Context, EffortCommand) InteractiveSessionState {
	return InteractiveSessionState{}
}
func (*contractStateController) UpdateCollaborationMode(context.Context, CollaborationCommand) InteractiveSessionState {
	return InteractiveSessionState{}
}
func (*contractStateController) Compact(context.Context, CompactCommand) (CompactOutcome, error) {
	return CompactOutcome{}, nil
}
func (*contractStateController) Rewind(context.Context, RewindCommand) RewindOutcome {
	return RewindOutcome{}
}
func (*contractStateController) RestoreLatestInput(context.Context) (RestoreInputOutcome, error) {
	return RestoreInputOutcome{}, nil
}
