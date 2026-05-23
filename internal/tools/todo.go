package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

// ReadTodoTool reads the current session TODO.md file.
type ReadTodoTool struct {
	todoPath string
}

// NewReadTodoTool creates a tool that reads only the session TODO.md file.
func NewReadTodoTool(sessionDir string) *ReadTodoTool {
	return &ReadTodoTool{todoPath: filepath.Join(sessionDir, "TODO.md")}
}

func (t *ReadTodoTool) Name() string {
	return "read_todo"
}

func (t *ReadTodoTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "Read the current session TODO.md checklist. This tool does not accept a path.",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t *ReadTodoTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	data, err := os.ReadFile(t.todoPath)
	if os.IsNotExist(err) {
		return "# TODO\n\n- [ ] Not recorded.\n", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取 TODO.md 失败: %w", err)
	}
	return string(data), nil
}

func (t *ReadTodoTool) ParallelSafe() bool {
	return true
}

// UpdateTodoTool replaces the current session TODO.md file.
type UpdateTodoTool struct {
	todoPath string
}

// NewUpdateTodoTool creates a tool that updates only the session TODO.md file.
func NewUpdateTodoTool(sessionDir string) *UpdateTodoTool {
	return &UpdateTodoTool{todoPath: filepath.Join(sessionDir, "TODO.md")}
}

func (t *UpdateTodoTool) Name() string {
	return "update_todo"
}

func (t *UpdateTodoTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "Replace the current session TODO.md checklist. Do not use file tools or bash to edit session TODO.md.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Complete Markdown content for the session TODO.md file.",
				},
			},
			"required": []string{"content"},
		},
	}
}

type updateTodoArgs struct {
	Content string `json:"content"`
}

func (t *UpdateTodoTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var input updateTodoArgs
	if err := json.Unmarshal(args, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	content := strings.TrimSpace(input.Content)
	if content == "" {
		return "", fmt.Errorf("TODO.md content cannot be empty")
	}
	content = ensureTrailingNewline(input.Content)
	if err := os.MkdirAll(filepath.Dir(t.todoPath), 0755); err != nil {
		return "", fmt.Errorf("创建 TODO.md 目录失败: %w", err)
	}
	if err := os.WriteFile(t.todoPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("写入 TODO.md 失败: %w", err)
	}
	done, total := countTodoCheckboxes(content)
	return fmt.Sprintf("TODO.md updated: %d/%d items complete.", done, total), nil
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func countTodoCheckboxes(content string) (done int, total int) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") {
			total++
			continue
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "- [x]") {
			total++
			done++
		}
	}
	return done, total
}
