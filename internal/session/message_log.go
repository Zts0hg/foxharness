package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Zts0hg/foxharness/internal/schema"
)

const (
	// MessageKindNormal is a regular model-visible conversation message.
	MessageKindNormal = "normal"
)

// MessageRecord is one line in messages.jsonl. It stores raw model-visible
// messages at session scope so future runs can continue from original context.
type MessageRecord struct {
	Seq            int64          `json:"seq"`
	RunID          string         `json:"run_id"`
	Time           time.Time      `json:"time"`
	Kind           string         `json:"kind,omitempty"`
	Message        schema.Message `json:"message"`
	DisplayContent string         `json:"display_content,omitempty"`

	IsMeta                    bool `json:"is_meta,omitempty"`
	IsCompactSummary          bool `json:"is_compact_summary,omitempty"`
	IsVisibleInTranscriptOnly bool `json:"is_visible_in_transcript_only,omitempty"`
}

// HumanContent returns the user-facing text for this record. It preserves the
// model-visible message content for old records and for messages that do not
// need a separate display form.
func (r MessageRecord) HumanContent() string {
	if strings.TrimSpace(r.DisplayContent) != "" {
		return r.DisplayContent
	}
	return r.Message.Content
}

// MessageLog manages a session's raw model-visible message history.
type MessageLog struct {
	path      string
	mu        sync.Mutex
	nextSeq   int64
	seqLoaded bool
}

// NewMessageLog creates a MessageLog for the provided session.
func NewMessageLog(s *Session) *MessageLog {
	return &MessageLog{path: s.MessagesPath()}
}

// Append records a normal model-visible message for a run and returns its
// assigned sequence number.
func (l *MessageLog) Append(runID string, msg schema.Message) (int64, error) {
	return l.AppendKind(runID, MessageKindNormal, msg)
}

// AppendWithDisplay records a normal model-visible message with an optional
// human-facing display form.
func (l *MessageLog) AppendWithDisplay(runID string, msg schema.Message, displayContent string) (int64, error) {
	return l.appendRecord(runID, MessageKindNormal, msg, displayContent)
}

// AppendKind records a model-visible message with a specific kind and returns
// its assigned sequence number.
func (l *MessageLog) AppendKind(runID, kind string, msg schema.Message) (int64, error) {
	return l.appendRecord(runID, kind, msg, "")
}

func (l *MessageLog) appendRecord(runID, kind string, msg schema.Message, displayContent string) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg = schema.NormalizeMessage(msg)
	if err := l.ensureSeqLoaded(); err != nil {
		return 0, err
	}
	seq := l.nextSeq
	l.nextSeq++

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("打开消息日志失败: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(MessageRecord{
		Seq:            seq,
		RunID:          runID,
		Time:           time.Now(),
		Kind:           kind,
		Message:        msg,
		DisplayContent: strings.TrimSpace(displayContent),
	})
	if err != nil {
		return 0, fmt.Errorf("序列化消息日志失败: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return 0, fmt.Errorf("写入消息日志失败: %w", err)
	}
	return seq, nil
}

// NextSeq returns the sequence number that will be assigned to the next
// appended message without mutating the log.
func (l *MessageLog) NextSeq() (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := l.ensureSeqLoaded(); err != nil {
		return 0, err
	}
	return l.nextSeq, nil
}

// TruncateBeforeSeq removes the selected message and all records after it.
// The next append reuses seq so a restored prompt can be submitted again.
func (l *MessageLog) TruncateBeforeSeq(seq int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	records, err := l.LoadRecords()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path+".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开临时消息日志失败: %w", err)
	}
	writeErr := func() error {
		defer f.Close()
		for _, record := range records {
			if record.Seq >= seq {
				continue
			}
			line, err := json.Marshal(record)
			if err != nil {
				return fmt.Errorf("序列化消息日志失败: %w", err)
			}
			if _, err := f.Write(append(line, '\n')); err != nil {
				return fmt.Errorf("写入消息日志失败: %w", err)
			}
		}
		return nil
	}()
	if writeErr != nil {
		_ = os.Remove(l.path + ".tmp")
		return writeErr
	}
	if err := os.Rename(l.path+".tmp", l.path); err != nil {
		_ = os.Remove(l.path + ".tmp")
		return fmt.Errorf("替换消息日志失败: %w", err)
	}
	l.nextSeq = seq
	l.seqLoaded = true
	return nil
}

// LoadRecords reads all message records in chronological order.
func (l *MessageLog) LoadRecords() ([]MessageRecord, error) {
	f, err := os.Open(l.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("打开消息日志失败: %w", err)
	}
	defer f.Close()

	var records []MessageRecord
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record MessageRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("解析消息日志失败: %w", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取消息日志失败: %w", err)
	}
	return records, nil
}

// LoadMessages reads model-visible messages in chronological order.
func (l *MessageLog) LoadMessages() ([]schema.Message, error) {
	records, err := l.LoadRecords()
	if err != nil {
		return nil, err
	}
	messages := make([]schema.Message, 0, len(records))
	for _, record := range records {
		messages = append(messages, record.Message)
	}
	return messages, nil
}

func (l *MessageLog) ensureSeqLoaded() error {
	if l.seqLoaded {
		return nil
	}
	records, err := l.LoadRecords()
	if err != nil {
		return err
	}
	var maxSeq int64
	for _, record := range records {
		if record.Seq >= maxSeq {
			maxSeq = record.Seq + 1
		}
	}
	l.nextSeq = maxSeq
	l.seqLoaded = true
	return nil
}
