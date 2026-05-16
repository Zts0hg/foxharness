package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// CompactState records how much raw message history has been summarized for a
// session. The raw messages remain in messages.jsonl; this state is only used
// to project a smaller model-facing context window on future runs.
type CompactState struct {
	Summary         string    `json:"summary"`
	CoveredUntilSeq int64     `json:"covered_until_seq"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// LoadCompactState loads the persisted compaction state for a session. Missing
// state returns an empty value.
func LoadCompactState(s *Session) (*CompactState, error) {
	data, err := os.ReadFile(s.CompactStatePath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &CompactState{CoveredUntilSeq: -1}, nil
		}
		return nil, fmt.Errorf("读取 Compact State 失败: %w", err)
	}

	var state CompactState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("解析 Compact State 失败: %w", err)
	}
	return &state, nil
}

// SaveCompactState persists the session compaction state.
func SaveCompactState(s *Session, state *CompactState) error {
	if state == nil {
		return nil
	}
	state.UpdatedAt = time.Now()
	if err := writeJSON(s.CompactStatePath(), state); err != nil {
		return fmt.Errorf("写入 Compact State 失败: %w", err)
	}
	return nil
}
