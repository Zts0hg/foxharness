package app

import (
	"context"
	"reflect"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

/* NotificationKind identifies one UI-neutral ordered run observation. */
type NotificationKind string

const (
	/* NotificationRunStarted marks the start of one run. */
	NotificationRunStarted NotificationKind = "run_started"
	/* NotificationMessage carries one complete assistant message. */
	NotificationMessage NotificationKind = "message"
	/* NotificationMessageDelta carries one streamed assistant text delta. */
	NotificationMessageDelta NotificationKind = "message_delta"
	/* NotificationThinking marks the thinking phase for one turn. */
	NotificationThinking NotificationKind = "thinking"
	/* NotificationContextCompacted marks one committed context reduction. */
	NotificationContextCompacted NotificationKind = "compaction"
	/* NotificationToolCall carries one ordered tool invocation. */
	NotificationToolCall NotificationKind = "tool_call"
	/* NotificationToolResult carries one ordered correlated tool result. */
	NotificationToolResult NotificationKind = "tool_result"
	/* NotificationRunCompleted marks successful terminal completion. */
	NotificationRunCompleted NotificationKind = "run_completed"
	/* NotificationRunError marks failed terminal completion. */
	NotificationRunError NotificationKind = "run_error"
)

/* Notification is the application-owned projection of one canonical runtime fact. */
type Notification struct {
	SessionID    string
	RunID        string
	Kind         NotificationKind
	Sequence     int
	Turn         int
	Phase        string
	CallID       string
	Name         string
	Content      string
	ArtifactPath string
	IsError      bool
}

/* MapRuntimeFact converts one runtime fact into its application-owned projection. */
func MapRuntimeFact(source foxruntime.RuntimeFact) Notification {
	return Notification{
		SessionID: string(source.SessionID), RunID: string(source.RunID),
		Kind: NotificationKind(source.Fact.Kind), Sequence: source.Fact.Sequence,
		Turn: source.Fact.Turn, Phase: string(source.Fact.Phase), CallID: source.Fact.CallID,
		Name: source.Fact.Name, Content: source.Fact.Content,
		ArtifactPath: source.Fact.ArtifactPath, IsError: source.Fact.IsError,
	}
}

/* MapRuntimeRunResult converts one runtime result into a defensive application DTO. */
func MapRuntimeRunResult(source foxruntime.RunResult) RunOutcome {
	var warnings []Warning
	if source.Warnings != nil {
		warnings = make([]Warning, len(source.Warnings))
		for index, warning := range source.Warnings {
			warnings[index] = Warning{Sink: warning.Sink, Operation: warning.Operation, Error: warning.Error}
		}
	}
	errorText := ""
	if source.Outcome.Err != nil {
		errorText = source.Outcome.Err.Error()
	}
	return RunOutcome{
		SessionID: string(source.SessionID), RunID: string(source.RunID),
		FinalMessage: source.Outcome.FinalMessage, CommittedMessage: source.CommittedMessage,
		FinishReason: source.Outcome.FinishReason, TurnCount: source.Outcome.TurnCount,
		Usage: Usage{
			InputTokens: source.Outcome.Usage.InputTokens, OutputTokens: source.Outcome.Usage.OutputTokens,
			CacheCreationTokens: source.Outcome.Usage.CacheCreationTokens,
			CacheReadTokens:     source.Outcome.Usage.CacheReadTokens,
		},
		Partial: source.Outcome.Partial, ErrorKind: source.Outcome.ErrorKind, Error: errorText,
		ArtifactPaths: append([]string(nil), source.ArtifactPaths...), Warnings: warnings,
	}
}

type runtimeNotificationObserver struct {
	sink NotificationSink
}

func (o runtimeNotificationObserver) ObserveRunFact(ctx context.Context, fact foxruntime.RuntimeFact) {
	o.sink.Notify(ctx, MapRuntimeFact(fact))
}

/* NewRuntimeNotificationObserver maps the canonical runtime stream to one application sink. */
func NewRuntimeNotificationObserver(sink NotificationSink) foxruntime.RunObserver {
	if isNilNotificationSink(sink) {
		return nil
	}
	return runtimeNotificationObserver{sink: sink}
}

func isNilNotificationSink(sink NotificationSink) bool {
	if sink == nil {
		return true
	}
	value := reflect.ValueOf(sink)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
