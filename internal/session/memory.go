package session

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func initialWorkingMemory() string {
	return strings.TrimSpace(`
# Working Memory

## Goal

未记录。

## Known Facts

未记录。

## Current Plan

未记录。

## Next Step

未记录。
`) + "\n"
}

type WorkingMemory struct {
	path string
}

func NewMemory(s *Session) *WorkingMemory {
	return &WorkingMemory{path: s.MemoryPath()}
}

func (m *WorkingMemory) Load() (string, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		return "", fmt.Errorf("读取 Working Memory 失败: %w", err)
	}
	return string(data), nil
}

func (m *WorkingMemory) Append(note string) error {
	note = strings.TrimSpace(note)
	if note == "" {
		return nil
	}
	entry := fmt.Sprintf(
		"\n## Note %s\n\n%s\n",
		time.Now().Format(time.RFC3339),
		note,
	)
	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("打开 Working Memory 失败: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(entry)
	return err
}

func (m *WorkingMemory) Replace(content string) error {
	return os.WriteFile(m.path, []byte(strings.TrimSpace(content)+"\n"), 0644)
}
