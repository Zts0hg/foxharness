package app

import (
	"context"
	"reflect"
	"testing"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

type interactiveRuntimeSessionStub struct {
	result foxruntime.RunResult
	specs  []foxruntime.RunSpec
}

func (s *interactiveRuntimeSessionStub) Run(_ context.Context, spec foxruntime.RunSpec) (foxruntime.RunResult, error) {
	s.specs = append(s.specs, spec)
	return s.result, nil
}

type interactivePermissionStub struct {
	state   PermissionState
	cleared int
}

func (s *interactivePermissionStub) PermissionState() PermissionState { return s.state }
func (s *interactivePermissionStub) UpdatePermissionMode(_ context.Context, command PermissionModeCommand) PermissionState {
	s.state.SelectedMode = command.Mode
	s.state.EffectiveMode = command.Mode
	return s.state
}
func (s *interactivePermissionStub) ActivateFullAccess(_ context.Context, command FullAccessCommand) PermissionState {
	s.state.SelectedMode = PermissionModeFullAccess
	s.state.EffectiveMode = PermissionModeFullAccess
	s.state.FullAccessRemembered = command.Remember
	return s.state
}
func (s *interactivePermissionStub) ClearPermissionGrants(context.Context) PermissionGrantClearOutcome {
	s.cleared++
	cleared := s.state.SessionGrantCount
	s.state.SessionGrantCount = 0
	return PermissionGrantClearOutcome{Cleared: cleared, State: s.state}
}

func TestInteractiveRuntimeApplicationOwnsRunSnapshotAndSessionSwitch(t *testing.T) {
	firstRuntime := &interactiveRuntimeSessionStub{result: foxruntime.RunResult{RunID: "run-1"}}
	secondRuntime := &interactiveRuntimeSessionStub{result: foxruntime.RunResult{RunID: "run-2"}}
	var order []string
	first := InteractiveRuntimeBinding{
		Session: firstRuntime,
		State: func() InteractiveSessionState {
			return InteractiveSessionState{Session: SessionInfo{ID: "session-1"}, WorkDir: "/work", ContextUsage: "10%"}
		},
		BeforeRun: func(context.Context, RunCommand) error { order = append(order, "before"); return nil },
		AfterRun:  func(context.Context, foxruntime.RunResult, error) { order = append(order, "after") },
		Close:     func(context.Context) error { order = append(order, "close-first"); return nil },
	}
	second := InteractiveRuntimeBinding{
		Session: secondRuntime,
		State: func() InteractiveSessionState {
			return InteractiveSessionState{Session: SessionInfo{ID: "session-2"}, WorkDir: "/work", ContextUsage: "0%"}
		},
	}
	permissions := &interactivePermissionStub{state: PermissionState{SessionGrantCount: 2}}
	var changedEffort string
	application, err := NewInteractiveRuntimeApplication(InteractiveRuntimeApplicationConfig{
		Initial: first,
		NewSession: func(context.Context) (InteractiveRuntimeBinding, error) {
			order = append(order, "create-second")
			return second, nil
		},
		RunSpec: foxruntime.RunSpec{ProviderProtocol: "openai"},
		Model:   "model-a", Effort: "high", CollaborationMode: "default",
		NormalizeCollaboration: func(value string) string { return "normalized:" + value },
		OnEffortChange:         func(value string) { changedEffort = value },
		Permissions:            permissions,
	})
	if err != nil {
		t.Fatal(err)
	}

	outcome, err := application.Run(context.Background(), RunCommand{
		Prompt: "work", DisplayPrompt: "display", AllowedTools: []string{"read_file"}, CollaborationMode: "formal_plan",
	}, nil)
	if err != nil || outcome == nil || outcome.RunID != "run-1" {
		t.Fatalf("run outcome/error = %#v/%v", outcome, err)
	}
	if len(firstRuntime.specs) != 1 {
		t.Fatalf("run specs = %#v", firstRuntime.specs)
	}
	gotSpec := firstRuntime.specs[0]
	if gotSpec.SessionID != "session-1" || gotSpec.Prompt != "work" || gotSpec.DisplayPrompt != "display" ||
		gotSpec.Model != "model-a" || gotSpec.Effort != "high" || gotSpec.CollaborationMode != "normalized:formal_plan" ||
		!reflect.DeepEqual(gotSpec.AllowedTools, []string{"read_file"}) {
		t.Fatalf("resolved run spec = %#v", gotSpec)
	}
	if !reflect.DeepEqual(order, []string{"before", "after"}) {
		t.Fatalf("run order = %#v", order)
	}

	state, err := application.NewSession(context.Background(), NewSessionCommand{})
	if err != nil || state.Session.ID != "session-2" {
		t.Fatalf("new session state/error = %#v/%v", state, err)
	}
	if !reflect.DeepEqual(order, []string{"before", "after", "create-second", "close-first"}) {
		t.Fatalf("session switch order = %#v", order)
	}
	if permissions.cleared != 1 || application.State().CollaborationMode != "normalized:default" {
		t.Fatalf("new-session reset: permission clears=%d state=%#v", permissions.cleared, application.State())
	}
	if state := application.UpdateEffort(context.Background(), EffortCommand{Effort: "low"}); changedEffort != "low" || state.Effort != "low" {
		t.Fatalf("effort propagation = %q/%#v", changedEffort, state)
	}
}

func TestInteractiveRuntimeApplicationDelegatesStateCapabilities(t *testing.T) {
	permissions := &interactivePermissionStub{state: PermissionState{SelectedMode: PermissionModeAsk}}
	binding := InteractiveRuntimeBinding{
		Session: &interactiveRuntimeSessionStub{},
		State: func() InteractiveSessionState {
			return InteractiveSessionState{Session: SessionInfo{ID: "session-1"}, Model: "backend-model"}
		},
		Conversation: func(context.Context) ([]ConversationRecord, error) {
			return []ConversationRecord{{Sequence: 1, Content: "message"}}, nil
		},
		ProjectInputHistory: func(context.Context, int) ([]string, error) { return []string{"history"}, nil },
		RewindTargets:       func(context.Context) ([]RewindTarget, error) { return []RewindTarget{{Sequence: 1}}, nil },
		Compact: func(context.Context, CompactCommand) (CompactOutcome, error) {
			return CompactOutcome{PreTokens: 10}, nil
		},
		Rewind:             func(context.Context, RewindCommand) RewindOutcome { return RewindOutcome{RestoredInput: "message"} },
		RestoreLatestInput: func(context.Context) (RestoreInputOutcome, error) { return RestoreInputOutcome{Restored: true}, nil },
	}
	application, err := NewInteractiveRuntimeApplication(InteractiveRuntimeApplicationConfig{
		Initial: binding, Model: "model-a", Effort: "medium", CollaborationMode: "default", Permissions: permissions,
	})
	if err != nil {
		t.Fatal(err)
	}
	if records, _ := application.Conversation(context.Background()); len(records) != 1 || records[0].Content != "message" {
		t.Fatalf("conversation = %#v", records)
	}
	if history, _ := application.ProjectInputHistory(context.Background(), 10); !reflect.DeepEqual(history, []string{"history"}) {
		t.Fatalf("history = %#v", history)
	}
	if targets, _ := application.RewindTargets(context.Background()); len(targets) != 1 || targets[0].Sequence != 1 {
		t.Fatalf("targets = %#v", targets)
	}
	if compacted, _ := application.Compact(context.Background(), CompactCommand{}); compacted.PreTokens != 10 {
		t.Fatalf("compact = %#v", compacted)
	}
	if rewound := application.Rewind(context.Background(), RewindCommand{}); rewound.RestoredInput != "message" {
		t.Fatalf("rewind = %#v", rewound)
	}
	if restored, _ := application.RestoreLatestInput(context.Background()); !restored.Restored {
		t.Fatalf("restore = %#v", restored)
	}
	if application.PermissionState().SelectedMode != PermissionModeAsk {
		t.Fatalf("permission state = %#v", application.PermissionState())
	}
	var _ InteractiveApplication = application
}

func TestInteractiveRuntimeApplicationUsesSelectedCollaborationWhenCommandOmitsOverride(t *testing.T) {
	runtimeSession := &interactiveRuntimeSessionStub{}
	application, err := NewInteractiveRuntimeApplication(InteractiveRuntimeApplicationConfig{
		Initial: InteractiveRuntimeBinding{
			Session: runtimeSession,
			State: func() InteractiveSessionState {
				return InteractiveSessionState{Session: SessionInfo{ID: "session-1"}}
			},
		},
		CollaborationMode: "formal_plan",
		NormalizeCollaboration: func(value string) string {
			if value == "formal_plan" {
				return value
			}
			return "default"
		},
		Permissions: &interactivePermissionStub{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := application.Run(context.Background(), RunCommand{Prompt: "work"}, nil); err != nil {
		t.Fatal(err)
	}
	if got := runtimeSession.specs[0].CollaborationMode; got != "formal_plan" {
		t.Fatalf("CollaborationMode = %q, want selected formal_plan", got)
	}
}
