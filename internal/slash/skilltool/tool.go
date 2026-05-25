package skilltool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/slash"
)

// SkillTool is the LLM-facing tool that lets the model invoke a prompt
// command registered in the slash registry. It exposes the unified
// execution pipeline (argument substitution, shell embedding, variable
// replacement, hooks, inline-or-fork dispatch) as a single tool call.
type SkillTool struct {
	registry  *slash.Registry
	executor  *slash.Executor
	sessionID func() string
}

// NewSkillTool constructs a SkillTool bound to the given registry and
// executor. sessionID is invoked at execution time so the tool can pick up
// the latest session identifier without holding a stale reference.
func NewSkillTool(registry *slash.Registry, executor *slash.Executor, sessionID func() string) *SkillTool {
	if sessionID == nil {
		sessionID = func() string { return "" }
	}
	return &SkillTool{registry: registry, executor: executor, sessionID: sessionID}
}

// Name returns the tool identifier "skill".
func (t *SkillTool) Name() string { return "skill" }

// Definition returns the tool schema for the LLM. The tool takes the
// skill's registered name and an optional argument string.
func (t *SkillTool) Definition() schema.ToolDefinition {
	return schema.ToolDefinition{
		Name:        t.Name(),
		Description: "Invoke a named skill registered in the slash command system. Pass the skill name (without leading slash) and a single arguments string.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{
					"type":        "string",
					"description": "Skill name (without leading slash). Must match an entry in the registered skill list.",
				},
				"arguments": map[string]interface{}{
					"type":        "string",
					"description": "Arguments string passed to the skill, parsed shell-style. May be empty.",
				},
			},
			"required": []string{"name"},
		},
	}
}

type skillToolArgs struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Execute resolves the requested skill and runs the executor pipeline.
// Returns the processed prompt body (inline mode) or the sub-agent's
// report (fork mode). Unknown skills and skills with
// `disable-model-invocation: true` return descriptive errors.
func (t *SkillTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	var args skillToolArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", fmt.Errorf("invalid skill arguments: %w", err)
	}
	if args.Name == "" {
		return "", fmt.Errorf("skill name is required")
	}
	if t.registry == nil {
		return "", fmt.Errorf("skill registry not configured")
	}
	cmd, ok := t.registry.Lookup(args.Name)
	if !ok {
		return "", fmt.Errorf("unknown skill: %q", args.Name)
	}
	if !cmd.IsModelInvocable() {
		return "", fmt.Errorf("skill %q is not model-invocable", args.Name)
	}
	if t.executor == nil {
		return "", fmt.Errorf("skill executor not configured")
	}
	res, err := t.executor.Execute(ctx, cmd, args.Arguments, t.sessionID())
	if err != nil {
		return "", err
	}
	return res.Content, nil
}
