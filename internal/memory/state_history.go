package memory

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrStateSnapshotNotFound indicates that no PLAN/TODO snapshot exists for a
// selected user message.
var ErrStateSnapshotNotFound = errors.New("session state snapshot not found")

// StateHistory stores rewind snapshots for session-local PLAN.md and TODO.md.
type StateHistory struct {
	store *Store
	path  string
	mu    sync.Mutex
}

type stateSnapshotRecord struct {
	MessageSeq int64     `json:"message_seq"`
	Plan       stateFile `json:"plan"`
	Todo       stateFile `json:"todo"`
	CreatedAt  time.Time `json:"created_at"`
}

type stateFile struct {
	Exists  bool   `json:"exists"`
	Content string `json:"content,omitempty"`
}

// NewStateHistory creates a PLAN/TODO snapshot history for the given store.
func NewStateHistory(store *Store) *StateHistory {
	return &StateHistory{
		store: store,
		path:  filepath.Join(store.planDir(), "state_history.jsonl"),
	}
}

// SnapshotBeforeMessage records the current PLAN.md and TODO.md state for the
// user message sequence. Existing snapshots are never overwritten.
func (h *StateHistory) SnapshotBeforeMessage(messageSeq int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	exists, err := h.hasSnapshotLocked(messageSeq)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	record := stateSnapshotRecord{
		MessageSeq: messageSeq,
		CreatedAt:  time.Now(),
	}
	record.Plan, err = readStateFile(h.store.PlanPath())
	if err != nil {
		return err
	}
	record.Todo, err = readStateFile(h.store.TodoPath())
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(h.path), 0755); err != nil {
		return fmt.Errorf("创建 session state 历史目录失败: %w", err)
	}
	f, err := os.OpenFile(h.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开 session state 历史失败: %w", err)
	}
	defer f.Close()

	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("序列化 session state 快照失败: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("写入 session state 快照失败: %w", err)
	}
	return nil
}

// RestoreBeforeMessage restores PLAN.md and TODO.md to the state captured
// before the selected user message.
func (h *StateHistory) RestoreBeforeMessage(messageSeq int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	record, err := h.snapshotLocked(messageSeq)
	if err != nil {
		return err
	}
	if err := restoreStateFile(h.store.PlanPath(), record.Plan); err != nil {
		return err
	}
	if err := restoreStateFile(h.store.TodoPath(), record.Todo); err != nil {
		return err
	}
	return nil
}

func (h *StateHistory) hasSnapshotLocked(messageSeq int64) (bool, error) {
	_, err := h.snapshotLocked(messageSeq)
	if errors.Is(err, ErrStateSnapshotNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (h *StateHistory) snapshotLocked(messageSeq int64) (stateSnapshotRecord, error) {
	f, err := os.Open(h.path)
	if errors.Is(err, os.ErrNotExist) {
		return stateSnapshotRecord{}, ErrStateSnapshotNotFound
	}
	if err != nil {
		return stateSnapshotRecord{}, fmt.Errorf("打开 session state 历史失败: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record stateSnapshotRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return stateSnapshotRecord{}, fmt.Errorf("解析 session state 历史失败: %w", err)
		}
		if record.MessageSeq == messageSeq {
			return record, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return stateSnapshotRecord{}, fmt.Errorf("读取 session state 历史失败: %w", err)
	}
	return stateSnapshotRecord{}, ErrStateSnapshotNotFound
}

func readStateFile(path string) (stateFile, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return stateFile{Exists: false}, nil
	}
	if err != nil {
		return stateFile{}, fmt.Errorf("读取 session state 文件失败: %w", err)
	}
	return stateFile{Exists: true, Content: string(data)}, nil
}

func restoreStateFile(path string, state stateFile) error {
	if !state.Exists {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("删除 session state 文件失败: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("创建 session state 文件目录失败: %w", err)
	}
	if err := os.WriteFile(path, []byte(state.Content), 0644); err != nil {
		return fmt.Errorf("恢复 session state 文件失败: %w", err)
	}
	return nil
}
