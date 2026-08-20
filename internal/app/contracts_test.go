package app

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
	"github.com/Zts0hg/foxharness/internal/schema"
)

type recordingNotificationSink struct {
	notifications []Notification
}

func (s *recordingNotificationSink) Notify(_ context.Context, notification Notification) {
	s.notifications = append(s.notifications, notification)
}

func TestApplicationMapsRuntimeFactsOnceIntoOwnedNotifications(t *testing.T) {
	sink := &recordingNotificationSink{}
	observer := NewRuntimeNotificationObserver(sink)
	observer.ObserveRunFact(context.Background(), foxruntime.RuntimeFact{
		SessionID: "session-1",
		RunID:     "run-1",
		Fact: engine.Fact{
			Kind: engine.FactToolResult, Sequence: 4, Turn: 2, Phase: engine.PhaseAction,
			CallID: "call-1", Name: "read_file", Content: "preview", FullContent: "complete",
			ArtifactPath: "/tmp/result", IsError: true,
		},
	})

	want := []Notification{{
		SessionID: "session-1", RunID: "run-1", Kind: NotificationToolResult,
		Sequence: 4, Turn: 2, Phase: "action", CallID: "call-1", Name: "read_file",
		Content: "preview", ArtifactPath: "/tmp/result", IsError: true,
	}}
	if !reflect.DeepEqual(sink.notifications, want) {
		t.Fatalf("notifications = %#v, want %#v", sink.notifications, want)
	}
}

func TestApplicationNotificationsDoNotExposeArtifactPayload(t *testing.T) {
	if _, exposed := reflect.TypeOf(Notification{}).FieldByName("FullContent"); exposed {
		t.Fatal("application notification exposes runtime artifact payload")
	}
}

func TestApplicationMapsRuntimeResultIntoDefensiveOwnedDTO(t *testing.T) {
	runtimeErr := errors.New("provider failed")
	source := foxruntime.RunResult{
		SessionID: "session-1", RunID: "run-1", CommittedMessage: "committed",
		Outcome: engine.RunOutcome{
			FinalMessage: "final", FinishReason: "stop", TurnCount: 3,
			Usage:   schema.Usage{InputTokens: 11, OutputTokens: 7, CacheCreationTokens: 5, CacheReadTokens: 2},
			Partial: true, ErrorKind: "provider", Err: runtimeErr,
		},
		ArtifactPaths: []string{"/tmp/artifact"},
		Warnings:      []foxruntime.RunWarning{{Sink: "telemetry", Operation: "write", Error: "offline"}},
	}

	got := MapRuntimeRunResult(source)
	source.ArtifactPaths[0] = "mutated"
	source.Warnings[0].Error = "mutated"

	want := RunOutcome{
		SessionID: "session-1", RunID: "run-1", FinalMessage: "final", CommittedMessage: "committed",
		FinishReason: "stop", TurnCount: 3,
		Usage:   Usage{InputTokens: 11, OutputTokens: 7, CacheCreationTokens: 5, CacheReadTokens: 2},
		Partial: true, ErrorKind: "provider", Error: "provider failed",
		ArtifactPaths: []string{"/tmp/artifact"},
		Warnings:      []Warning{{Sink: "telemetry", Operation: "write", Error: "offline"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("run outcome = %#v, want %#v", got, want)
	}
}

func TestApplicationRuntimeMappingPreservesAbsentCollectionsAndSink(t *testing.T) {
	got := MapRuntimeRunResult(foxruntime.RunResult{})
	if got.ArtifactPaths != nil || got.Warnings != nil {
		t.Fatalf("absent runtime collections mapped to %#v/%#v, want nil", got.ArtifactPaths, got.Warnings)
	}
	if observer := NewRuntimeNotificationObserver(nil); observer != nil {
		t.Fatalf("nil sink observer = %#v, want nil", observer)
	}
	var typedNil *recordingNotificationSink
	if observer := NewRuntimeNotificationObserver(typedNil); observer != nil {
		t.Fatalf("typed-nil sink observer = %#v, want nil", observer)
	}
}

type runUseCaseFunc func(context.Context, RunCommand, NotificationSink) (RunOutcome, error)

func (f runUseCaseFunc) Run(ctx context.Context, command RunCommand, sink NotificationSink) (RunOutcome, error) {
	return f(ctx, command, sink)
}

func TestRunUseCaseUsesOnlyApplicationCommandOutcomeAndNotifications(t *testing.T) {
	var _ RunUseCase = runUseCaseFunc(nil)
	command := RunCommand{
		Prompt: "inspect", DisplayPrompt: "Inspect", AllowedTools: []string{"read_file"},
		CollaborationMode: "default", Model: "model", Effort: "high",
	}
	useCase := runUseCaseFunc(func(_ context.Context, got RunCommand, _ NotificationSink) (RunOutcome, error) {
		if !reflect.DeepEqual(got, command) {
			t.Fatalf("command = %#v, want %#v", got, command)
		}
		return RunOutcome{SessionID: "session-1", RunID: "run-1", FinalMessage: "done"}, nil
	})

	result, err := useCase.Run(context.Background(), command, &recordingNotificationSink{})
	if err != nil || result.FinalMessage != "done" {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
}
