package session

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Transcript manages the conversation transcript for a session.
// The transcript is a JSONL file containing a chronological record of all
// events that occurred during the session.
type Transcript struct {
	// path is the file path to the transcript file.
	path string
}

// NewTranscript creates a new Transcript for the given session.
// Returns a Transcript that operates on the session's transcript file.
func NewTranscript(s *Session) *Transcript {
	return &Transcript{path: s.TranscriptPath()}
}

// Event represents a single entry in the transcript.
type Event struct {
	// Time is when the event occurred.
	Time time.Time `json:"time"`
	// Type identifies the kind of event (e.g., "user_prompt", "tool_call").
	Type string `json:"type"`
	// Payload contains the event-specific data.
	Payload any `json:"payload"`
}

// Append adds a new event to the transcript.
// The event is serialized as JSON and appended to the transcript file.
// Returns an error if the file cannot be written.
func (t *Transcript) Append(eventType string, payload any) error {
	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open transcript: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(Event{
		Time:    time.Now(),
		Type:    eventType,
		Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal transcript event: %w", err)
	}

	_, err = f.Write(append(line, '\n'))
	return err
}
