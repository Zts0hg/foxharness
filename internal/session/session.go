// Package session provides session lifecycle management for the foxharness agent.
//
// Each session represents a continuous conversation with its own isolated
// workspace containing memory, raw message history, transcript, metrics, and
// tracing data. A session may contain multiple runs, where each run represents
// one user-submitted task or message.
//
// Key Components:
//   - Manager: Creates and manages sessions
//   - Session: Represents a continuous conversation with metadata
//   - Run: Represents one user-submitted task within a session
//   - Transcript: Records conversation history
//   - Memory: Manages working memory state
//
// Session Structure:
//
//	.foxharness/sessions/{id}/
//	  ├── session.json      - Session metadata
//	  ├── working_memory.md - Working memory for the agent
//	  ├── messages.jsonl    - Raw model-visible message history
//	  ├── transcript.jsonl  - Session event log
//	  ├── artifacts/        - Files created during the session
//	  └── runs/{run-id}/
//	      ├── run.json      - Run metadata
//	      ├── metrics.jsonl - Token usage and performance metrics
//	      ├── trace.jsonl   - Span-based tracing
//	      └── artifacts/    - Files created during the run
package session

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Source identifies where a session request originated.
type Source string

const (
	// SOURCECLI indicates the session was initiated from the CLI.
	SOURCECLI Source = "cli"
	// SOURCEFeishu indicates the session was initiated from Feishu/Lark.
	SOURCEFeishu Source = "feishu"
	// SOURCESubagent indicates the session was initiated by a subagent.
	SOURCESubagent Source = "subagent"
)

// ErrNotFound indicates that a requested session could not be found.
var ErrNotFound = errors.New("session not found")

// Session represents a continuous agent conversation with its isolated
// workspace. Each session has a unique ID and may contain multiple runs.
type Session struct {
	// ID is the unique session identifier (format: YYYYMMDD-HHMMSS-random).
	ID string `json:"id"`
	// Source indicates where the session request originated.
	Source Source `json:"source"`
	// WorkDir is the working directory for file operations during execution.
	WorkDir string `json:"work_dir"`
	// RootDir is the session's root directory containing all artifacts.
	RootDir string `json:"root_dir"`
	// UserID optionally identifies the user who initiated the session.
	UserID string `json:"user_id,omitempty"`
	// ChatID optionally identifies the conversation (for chat platforms).
	ChatID string `json:"chat_id,omitempty"`
	// CreatedAt is the timestamp when the session was created.
	CreatedAt time.Time `json:"created_at"`
}

// MemoryPath returns the path to the working memory file for this session.
func (s *Session) MemoryPath() string {
	return filepath.Join(s.RootDir, "working_memory.md")
}

// TranscriptPath returns the path to the transcript file for this session.
func (s *Session) TranscriptPath() string {
	return filepath.Join(s.RootDir, "transcript.jsonl")
}

// MessagesPath returns the path to the raw model-visible message log.
func (s *Session) MessagesPath() string {
	return filepath.Join(s.RootDir, "messages.jsonl")
}

// ArtifactsDir returns the directory path for session artifacts.
func (s *Session) ArtifactsDir() string {
	return filepath.Join(s.RootDir, "artifacts")
}

// RunsDir returns the directory path for run-specific artifacts.
func (s *Session) RunsDir() string {
	return filepath.Join(s.RootDir, "runs")
}

// MetricsPath returns the path to the metrics file for this session.
func (s *Session) MetricsPath() string {
	return filepath.Join(s.RootDir, "metrics.jsonl")
}

// TracePath returns the path to the tracing file for this session.
func (s *Session) TracePath() string {
	return filepath.Join(s.RootDir, "trace.jsonl")
}

// Manager creates and manages agent sessions.
// All sessions are stored under a base directory with unique IDs.
type Manager struct {
	// baseDir is the root directory for all sessions.
	baseDir string
}

// NewManager creates a new Manager that stores sessions under the provided working directory.
// Sessions are stored in {workDir}/.foxharness/sessions/{session-id}/.
// Returns a configured Manager.
func NewManager(workDir string) *Manager {
	return &Manager{
		baseDir: filepath.Join(workDir, ".foxharness", "sessions"),
	}
}

// CreateOptions configures the creation of a new session.
type CreateOptions struct {
	// Source identifies where the session request originated.
	Source Source
	// WorkDir is the working directory for file operations during execution.
	WorkDir string
	// UserID optionally identifies the user who initiated the session.
	UserID string
	// ChatID optionally identifies the conversation (for chat platforms).
	ChatID string
}

// LookupOptions filters sessions while searching for the most recent matching
// conversation. Empty fields are ignored.
type LookupOptions struct {
	Source Source
	UserID string
	ChatID string
}

// Create creates a new session with the provided options.
// A unique session ID is generated, and the session directory structure
// is initialized with all required files.
// Returns the created Session, or an error if initialization fails.
func (m *Manager) Create(opts CreateOptions) (*Session, error) {
	id := newSessionID()
	root := filepath.Join(m.baseDir, id)

	if err := os.MkdirAll(filepath.Join(root, "artifacts"), 0755); err != nil {
		return nil, fmt.Errorf("创建 Session 目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "runs"), 0755); err != nil {
		return nil, fmt.Errorf("创建 Run 目录失败: %w", err)
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
	if err := os.WriteFile(s.MessagesPath(), nil, 0644); err != nil {
		return nil, fmt.Errorf("初始化消息日志失败: %w", err)
	}

	return s, nil
}

// Open loads an existing session by ID.
func (m *Manager) Open(id string) (*Session, error) {
	path := filepath.Join(m.baseDir, id, "session.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("读取 Session 失败: %w", err)
	}

	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("解析 Session 失败: %w", err)
	}
	if s.RootDir == "" {
		s.RootDir = filepath.Join(m.baseDir, id)
	}
	if s.ID == "" {
		s.ID = id
	}
	return &s, nil
}

// Latest returns the most recently created session matching opts.
func (m *Manager) Latest(opts LookupOptions) (*Session, error) {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("读取 Session 目录失败: %w", err)
	}

	var latest *Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		s, err := m.Open(entry.Name())
		if err != nil {
			continue
		}
		if !matchesLookup(s, opts) {
			continue
		}
		if latest == nil || s.CreatedAt.After(latest.CreatedAt) {
			latest = s
		}
	}
	if latest == nil {
		return nil, ErrNotFound
	}
	return latest, nil
}

func matchesLookup(s *Session, opts LookupOptions) bool {
	if opts.Source != "" && s.Source != opts.Source {
		return false
	}
	if opts.UserID != "" && s.UserID != opts.UserID {
		return false
	}
	if opts.ChatID != "" && s.ChatID != opts.ChatID {
		return false
	}
	return true
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
