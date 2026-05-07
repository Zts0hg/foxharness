package subagent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
)

type Tool struct {
	manager         *Manager
	ParentSessionID string
}

func NewTool(manager *Manager, parentSessionID string) *Tool {
	return &Tool{
		manager:         manager,
		ParentSessionID: parentSessionID,
	}
}

func (t *Tool) Name() string {
	return "delegate_task"
}

func (t *Tool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "将一个边界清晰、可独立探索的子任务委派给独立 Subagent。适合宽搜索、只读分析、调用关系梳理。不要用于需要立刻修改文件的任务。",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"task": map[string]interface{}{
					"type":        "string",
					"description": "要委派的子任务，必须明确输入、范围和期望输出。",
				},
				"read_only": map[string]interface{}{
					"type":        "boolean",
					"description": "是否限制 Subagent 只能做只读探索，默认 true。",
				},
			},
			"required": []string{"task"},
		},
	}
}

type args struct {
	Task     string `json:"task"`
	ReadOnly *bool  `json:"read_only"`
}

func (t *Tool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var input args
	if err := json.Unmarshal(raw, &input); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	input.Task = strings.TrimSpace(input.Task)
	if input.Task == "" {
		return "", fmt.Errorf("task 不能为空")
	}

	readOnly := true
	if input.ReadOnly != nil {
		readOnly = *input.ReadOnly
	}

	result, err := t.manager.Run(ctx, Request{
		ParentSessionID: t.ParentSessionID,
		Task:            input.Task,
		ReadOnly:        readOnly,
	})

	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"Subagent Session: %s\n\nReport:\n%s",
		result.SessionID,
		result.Report,
	), nil
}
