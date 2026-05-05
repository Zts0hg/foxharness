package session

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Transcript struct {
	path string
}

func NewTranscript(s *Session) *Transcript {
	return &Transcript{path: s.TranscriptPath()}
}

type Event struct {
	Time    time.Time `json:"time"`
	Type    string    `json:"type"`
	Payload any       `json:"payload"`
}

func (t *Transcript) Append(eventType string, payload any) error {
	f, err := os.OpenFile(t.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开 transcript 失败: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(Event{
		Time:    time.Now(),
		Type:    eventType,
		Payload: payload,
	})
	if err != nil {
		return fmt.Errorf("序列化 transcript 事件失败: %w", err)
	}

	_, err = f.Write(append(line, '\n'))
	return err
}
