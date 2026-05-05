package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Source string

const (
	SOURCECLI    Source = "cli"
	SOURCEFeishu Source = "feishu"
)

type Session struct {
	ID        string    `json:"id"`
	Source    Source    `json:"source"`
	WorkDir   string    `json:"work_dir"`
	RootDir   string    `json:"root_dir"`
	UserID    string    `json:"user_id,omitempty"`
	ChatID    string    `json:"chat_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Session) MemoryPath() string {
	return filepath.Join(s.RootDir, "working_memory.md")
}

func (s *Session) TranscriptPath() string {
	return filepath.Join(s.RootDir, "transcript.jsonl")
}

func (s *Session) ArtifactsDir() string {
	return filepath.Join(s.RootDir, "artifacts")
}

type Manager struct {
	baseDir string
}

func NewManager(workDir string) *Manager {
	return &Manager{
		baseDir: filepath.Join(workDir, ".foxharness", "sessions"),
	}
}

type CreateOptions struct {
	Source  Source
	WorkDir string
	UserID  string
	ChatID  string
}

func (m *Manager) Create(opts CreateOptions) (*Session, error) {
	id := newSessionID()
	root := filepath.Join(m.baseDir, id)

	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0755); err != nil {
		return nil, fmt.Errorf("创建 Session 目录失败: %w", err)
	}

	s := &Session{
		ID:        id,
		Source:    opts.Source,
		WorkDir:   opts.WorkDir,
		RootDir:   root,
		UserID:    opts.UserID,
		ChatID:    opts.ChatID,
		CreatedAt: time.Now(),
	}
	if err := writeJSON(filepath.Join(root, "session.json"), s); err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.MemoryPath(), []byte(initialWorkingMemory()), 0644); err != nil {
		return nil, fmt.Errorf("初始化 Working Memory 失败: %w", err)
	}

	return s, nil
}

func newSessionID() string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return time.Now().Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失败: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
