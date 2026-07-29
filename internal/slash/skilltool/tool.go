package skilltool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
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

// AssessPermission plans the exact skill pipeline without executing shell,
// hooks, or forked work.
func (t *SkillTool) AssessPermission(ctx toolpolicy.Context, raw json.RawMessage) (toolpolicy.Assessment, error) {
	args, cmd, err := t.resolve(raw)
	if err != nil || t.executor == nil {
		return skillHumanOnly(t.Name(), firstError(err, "skill executor not configured")), nil
	}
	plan, err := t.executor.Plan(cmd, args.Arguments, t.sessionID())
	if err != nil {
		return skillHumanOnly(t.Name()+" "+args.Name, err.Error()), nil
	}
	assessment := toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorReviewable,
		Action:   skillAction(t.Name(), args, plan.Commands),
		Effects:  []toolpolicy.Effect{toolpolicy.EffectWorkflow},
		Scope:    toolpolicy.ScopeWorkspace,
		ReadOnly: true,
		RiskHint: toolpolicy.RiskLow,
		Reason:   "model-invoked skill requires contextual review",
		Target:   args.Name,
		Commands: append([]string(nil), plan.Commands...),
	}
	if len(plan.Commands) > 0 {
		assessment.Effects = append(assessment.Effects, toolpolicy.EffectExecute)
		assessment.Scope = toolpolicy.ScopeMixed
		for _, command := range plan.Commands {
			readOnly, risk, parsed := toolpolicy.AssessShell(command, ctx.Workspace, ctx.CWD)
			if !parsed {
				return asSkillHumanOnly(assessment, "skill shell syntax could not be parsed"), nil
			}
			assessment.ReadOnly = assessment.ReadOnly && readOnly
			assessment.RiskHint = maxRisk(assessment.RiskHint, risk)
		}
		assessment.Reason = "skill executes planned shell commands"
	}
	if plan.Fork {
		assessment.Effects = append(assessment.Effects, toolpolicy.EffectDelegate)
		assessment.NestedEnforcement = t.executor.ForkPermissionEnforced()
		if !assessment.NestedEnforcement {
			return asSkillHumanOnly(assessment, "forked skill lacks nested permission enforcement"), nil
		}
		assessment.ReadOnly = false
		assessment.RiskHint = maxRisk(assessment.RiskHint, toolpolicy.RiskMedium)
		assessment.Reason = "forked skill inherits nested permission enforcement"
	}
	return assessment, nil
}

func skillAction(toolName string, args skillToolArgs, commands []string) string {
	action := strings.TrimSpace(fmt.Sprintf("%s %s %s", toolName, args.Name, args.Arguments))
	if len(commands) > 0 {
		action += fmt.Sprintf("; planned commands=%q", commands)
	}
	return action
}

// Execute resolves the requested skill and runs the executor pipeline.
// Returns the processed prompt body (inline mode) or the sub-agent's
// report (fork mode). Unknown skills and skills with
// `disable-model-invocation: true` return descriptive errors.
func (t *SkillTool) Execute(ctx context.Context, raw json.RawMessage) (string, error) {
	args, cmd, err := t.resolve(raw)
	if err != nil {
		return "", err
	}
	if t.executor == nil {
		return "", fmt.Errorf("skill executor not configured")
	}
	res, err := t.executor.Execute(ctx, cmd, args.Arguments, t.sessionID())
	if err != nil {
		return "", err
	}
	if res.AfterHook != nil {
		defer res.AfterHook(ctx)
	}
	return res.Content, nil
}

func (t *SkillTool) resolve(raw json.RawMessage) (skillToolArgs, *slash.Command, error) {
	var args skillToolArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return skillToolArgs{}, nil, fmt.Errorf("invalid skill arguments: %w", err)
	}
	if args.Name == "" {
		return skillToolArgs{}, nil, fmt.Errorf("skill name is required")
	}
	if t.registry == nil {
		return skillToolArgs{}, nil, fmt.Errorf("skill registry not configured")
	}
	cmd, ok := t.registry.Lookup(args.Name)
	if !ok {
		return skillToolArgs{}, nil, fmt.Errorf("unknown skill: %q", args.Name)
	}
	if !cmd.IsModelInvocable() {
		return skillToolArgs{}, nil, fmt.Errorf("skill %q is not model-invocable", args.Name)
	}
	if len(cmd.Frontmatter.AllowedTools) > 0 && cmd.Frontmatter.Context != "fork" {
		return skillToolArgs{}, nil, fmt.Errorf("skill %q declares allowed-tools=%v but context=inline; "+
			"model-side enforcement requires context: fork — change the skill's frontmatter or invoke from the TUI",
			args.Name, cmd.Frontmatter.AllowedTools)
	}
	return args, cmd, nil
}

func skillHumanOnly(action, reason string) toolpolicy.Assessment {
	return toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorHumanOnly,
		Action:   action,
		Effects:  []toolpolicy.Effect{toolpolicy.EffectUnknown},
		Scope:    toolpolicy.ScopeUnknown,
		RiskHint: toolpolicy.RiskHigh,
		Reason:   reason,
	}
}

func asSkillHumanOnly(assessment toolpolicy.Assessment, reason string) toolpolicy.Assessment {
	assessment.Behavior = toolpolicy.BehaviorHumanOnly
	assessment.RiskHint = toolpolicy.RiskHigh
	assessment.Reason = reason
	return assessment
}

func firstError(err error, fallback string) string {
	if err != nil {
		return err.Error()
	}
	return fallback
}

func maxRisk(left, right toolpolicy.Risk) toolpolicy.Risk {
	rank := map[toolpolicy.Risk]int{
		toolpolicy.RiskLow: 1, toolpolicy.RiskMedium: 2,
		toolpolicy.RiskHigh: 3, toolpolicy.RiskCritical: 4,
	}
	if rank[right] > rank[left] {
		return right
	}
	return left
}
