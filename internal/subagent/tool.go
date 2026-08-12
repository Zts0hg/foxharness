package subagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

// Tool implements the agent tool interface for delegating tasks to a
// subagent. It is registered under the name "delegate_task" and accepts a
// task description and optional read-only flag as JSON input.
type Tool struct {
	manager         *Manager
	ParentSessionID string
}

// NewTool creates a delegate_task tool backed by the given Manager and
// associated with the specified parent session.
func NewTool(manager *Manager, parentSessionID string) *Tool {
	return &Tool{
		manager:         manager,
		ParentSessionID: parentSessionID,
	}
}

// Name returns the tool identifier "delegate_task".
func (t *Tool) Name() string {
	return "delegate_task"
}

// Definition returns the JSON schema definition for the delegate_task tool,
// describing its parameters and usage constraints.
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

// AssessPermission describes delegation while preserving Execute's read-only
// default and nested permission boundary.
func (t *Tool) AssessPermission(_ toolpolicy.Context, raw json.RawMessage) (toolpolicy.Assessment, error) {
	var input args
	if err := json.Unmarshal(raw, &input); err != nil || strings.TrimSpace(input.Task) == "" {
		return toolpolicy.Assessment{
			Behavior: toolpolicy.BehaviorHumanOnly,
			Action:   t.Name(),
			Effects:  []toolpolicy.Effect{toolpolicy.EffectUnknown},
			Scope:    toolpolicy.ScopeUnknown,
			RiskHint: toolpolicy.RiskHigh,
			Reason:   "invalid or missing delegated task",
		}, nil
	}
	readOnly := true
	if input.ReadOnly != nil {
		readOnly = *input.ReadOnly
	}
	nested := t.manager != nil && t.manager.permissions != nil
	assessment := toolpolicy.Assessment{
		Behavior:          toolpolicy.BehaviorReviewable,
		Action:            fmt.Sprintf("%s read_only=%t task=%s", t.Name(), readOnly, strings.TrimSpace(input.Task)),
		Effects:           []toolpolicy.Effect{toolpolicy.EffectDelegate},
		Scope:             toolpolicy.ScopeWorkspace,
		ReadOnly:          readOnly,
		NestedEnforcement: nested,
		RiskHint:          toolpolicy.RiskLow,
		Reason:            "read-only delegation with nested permission enforcement",
		Target:            strings.TrimSpace(input.Task),
	}
	if !readOnly {
		assessment.RiskHint = toolpolicy.RiskMedium
		assessment.Reason = "writable delegation with nested permission enforcement"
	}
	if !nested {
		assessment.Behavior = toolpolicy.BehaviorHumanOnly
		assessment.RiskHint = toolpolicy.RiskHigh
		assessment.Reason = "delegation lacks nested permission enforcement"
	}
	return assessment, nil
}

// Execute parses the JSON input, delegates the task to the subagent Manager,
// and returns the session ID and report. It defaults to read-only mode when
// the read_only field is omitted.
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

	invocation, _ := tools.InvocationContextFrom(ctx)
	result, err := t.manager.Run(ctx, Request{
		ParentSessionID: t.ParentSessionID,
		ParentRunID:     invocation.RunID,
		DelegationID:    invocation.ToolCallID,
		Task:            input.Task,
		ReadOnly:        readOnly,
		Depth:           1,
	})

	if err != nil {
		return FormatFailureOutcome(result, err), &OutcomeError{Outcome: result, Err: err}
	}

	return fmt.Sprintf(
		"Subagent Session: %s\n\nReport:\n%s",
		result.SessionID,
		result.Report,
	), nil
}

// ExecuteResult preserves a failed ChildRun's correlated outcome in the model-
// visible tool result while marking it as a failure.
func (t *Tool) ExecuteResult(ctx context.Context, raw json.RawMessage) (tools.ExecutionResult, error) {
	output, err := t.Execute(ctx, raw)
	if err == nil {
		return tools.ExecutionResult{Output: output}, nil
	}
	var outcomeErr *OutcomeError
	if errors.As(err, &outcomeErr) {
		return tools.ExecutionResult{Output: output, Failed: true}, nil
	}
	return tools.ExecutionResult{}, err
}
