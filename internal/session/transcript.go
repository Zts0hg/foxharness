package session

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// TranscriptLog appends persisted observation artifacts to a session JSONL
// transcript. It is not authoritative recoverable session state.
type TranscriptLog struct {
	// path is the file path to the transcript file.
	path string
}

// Transcript is the deprecated compatibility name for TranscriptLog.
// Deprecated: use TranscriptLog.
type Transcript = TranscriptLog

// NewTranscriptLog creates a TranscriptLog for the provided stored session.
func NewTranscriptLog(s *StoredSession) *TranscriptLog {
	return &TranscriptLog{path: s.TranscriptPath()}
}

// NewTranscript is the deprecated compatibility constructor for
// NewTranscriptLog.
// Deprecated: use NewTranscriptLog.
func NewTranscript(s *StoredSession) *Transcript {
	return NewTranscriptLog(s)
}

// TranscriptEvent represents one persisted transcript artifact.
type TranscriptEvent struct {
	// Time is when the event occurred.
	Time time.Time `json:"time"`
	// RunID identifies the run that produced this event, when applicable.
	RunID RunID `json:"run_id,omitempty"`
	// Type identifies the kind of event (e.g., "user_prompt", "tool_call").
	Type string `json:"type"`
	// Payload contains the event-specific data.
	Payload any `json:"payload"`
}

// Event is the deprecated compatibility name for TranscriptEvent.
// Deprecated: use TranscriptEvent.
type Event = TranscriptEvent

// Append adds a new event to the transcript.
// The event is serialized as JSON and appended to the transcript file.
// Returns an error if the file cannot be written.
func (t *TranscriptLog) Append(eventType string, payload any) error {
	return t.AppendRun("", eventType, payload)
}

// AppendRun adds a new run-scoped event to the transcript.
func (t *TranscriptLog) AppendRun(runID RunID, eventType string, payload any) error {
	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transcript: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(TranscriptEvent{
		Time:    time.Now(),
		RunID:   runID,
		Type:    eventType,
		Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal transcript event: %w", err)
	}

	_, err = f.Write(append(line, '\n'))
	return err
}
