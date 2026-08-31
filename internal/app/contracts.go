package app

import "context"

/* RunCommand is the UI-neutral input for one user-entry agent invocation. */
type RunCommand struct {
	Prompt            string
	DisplayPrompt     string
	AllowedTools      []string
	CollaborationMode string
	Model             string
	Effort            string
}

/* Usage is application-owned normalized model token accounting. */
type Usage struct {
	InputTokens         int64
	OutputTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

/* Warning is a non-terminal application-visible runtime sink failure. */
type Warning struct {
	Sink      string
	Operation string
	Error     string
}

/* SessionInfo is the presentation-safe identity and artifact location of one selected session. */
type SessionInfo struct {
	ID             string
	Directory      string
	TranscriptPath string
}

/* RunOutcome is the UI-neutral terminal projection of one runtime run. */
type RunOutcome struct {
	SessionID        string
	RunID            string
	FinalMessage     string
	CommittedMessage string
	FinishReason     string
	TurnCount        int
	Usage            Usage
	Partial          bool
	ErrorKind        string
	Error            string
	MetricsPath      string
	TracePath        string
	ArtifactPaths    []string
	Warnings         []Warning
}

/* RunUseCase submits one command and synchronously returns its terminal projection. */
type RunUseCase interface {
	Run(context.Context, RunCommand, NotificationSink) (*RunOutcome, error)
}

/* NotificationSink synchronously consumes ordered application notifications. */
type NotificationSink interface {
	Notify(context.Context, Notification)
}
