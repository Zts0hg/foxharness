package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

func (t *ReadFileTool) AssessPermission(ctx toolpolicy.Context, args json.RawMessage) (toolpolicy.Assessment, error) {
	return assessFileCall(ctx, t.workDir, t.Name(), args, toolpolicy.EffectObserve, true, toolpolicy.RiskLow)
}

func (t *WriteFileTool) AssessPermission(ctx toolpolicy.Context, args json.RawMessage) (toolpolicy.Assessment, error) {
	return assessFileCall(ctx, t.workDir, t.Name(), args, toolpolicy.EffectMutate, false, toolpolicy.RiskMedium)
}

func (t *EditFileTool) AssessPermission(ctx toolpolicy.Context, args json.RawMessage) (toolpolicy.Assessment, error) {
	return assessFileCall(ctx, t.workDir, t.Name(), args, toolpolicy.EffectMutate, false, toolpolicy.RiskMedium)
}

func assessFileCall(ctx toolpolicy.Context, workDir, name string, args json.RawMessage, effect toolpolicy.Effect, readOnly bool, workspaceRisk toolpolicy.Risk) (toolpolicy.Assessment, error) {
	var input struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &input); err != nil || strings.TrimSpace(input.Path) == "" {
		return humanOnlyAssessment(name, "invalid or missing file path"), nil
	}
	workspace := firstPolicyValue(ctx.Workspace, workDir)
	cwd := firstPolicyValue(ctx.CWD, workDir, workspace)
	scope, ok := toolpolicy.PathScope(workspace, cwd, input.Path)
	if !ok {
		return humanOnlyAssessment(fmt.Sprintf("%s %s", name, input.Path), "file scope could not be proven"), nil
	}
	assessment := toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorFastAllow,
		Action:   fmt.Sprintf("%s %s", name, input.Path),
		Effects:  []toolpolicy.Effect{effect},
		Scope:    scope,
		ReadOnly: readOnly,
		RiskHint: workspaceRisk,
		Reason:   "workspace-contained file invocation",
		Target:   input.Path,
	}
	if scope != toolpolicy.ScopeWorkspace {
		assessment.Behavior = toolpolicy.BehaviorReviewable
		assessment.RiskHint = toolpolicy.RiskHigh
		assessment.Reason = "file target is outside the workspace"
	}
	return assessment, nil
}

func (t *BashTool) AssessPermission(ctx toolpolicy.Context, args json.RawMessage) (toolpolicy.Assessment, error) {
	var input bashArgs
	if err := json.Unmarshal(args, &input); err != nil || strings.TrimSpace(input.Command) == "" {
		return humanOnlyAssessment("bash", "invalid or missing shell command"), nil
	}
	command := strings.TrimSpace(input.Command)
	workspace := firstPolicyValue(ctx.Workspace, t.workDir)
	cwd := firstPolicyValue(ctx.CWD, t.workDir, workspace)
	readOnly, risk, parsed := toolpolicy.AssessShell(command, workspace, cwd)
	if !parsed {
		return humanOnlyAssessment("bash "+command, "shell syntax could not be parsed"), nil
	}
	assessment := toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorReviewable,
		Action:   "bash " + command,
		Effects:  []toolpolicy.Effect{toolpolicy.EffectExecute},
		Scope:    toolpolicy.ScopeMixed,
		ReadOnly: readOnly,
		RiskHint: risk,
		Reason:   "shell invocation requires contextual review",
		Commands: []string{command},
	}
	if readOnly {
		assessment.Behavior = toolpolicy.BehaviorFastAllow
		assessment.Scope = toolpolicy.ScopeWorkspace
		assessment.Reason = "read-only workspace shell invocation"
	}
	return assessment, nil
}

func (t *AskUserQuestionTool) AssessPermission(toolpolicy.Context, json.RawMessage) (toolpolicy.Assessment, error) {
	return trustedSessionAssessment(t.Name(), toolpolicy.EffectInteract), nil
}

func (t *ReadTodoTool) AssessPermission(toolpolicy.Context, json.RawMessage) (toolpolicy.Assessment, error) {
	return trustedSessionAssessment(t.Name(), toolpolicy.EffectObserve), nil
}

func (t *UpdateTodoTool) AssessPermission(toolpolicy.Context, json.RawMessage) (toolpolicy.Assessment, error) {
	return trustedSessionAssessment(t.Name(), toolpolicy.EffectWorkflow), nil
}

func (t *SubmitPlanTool) AssessPermission(toolpolicy.Context, json.RawMessage) (toolpolicy.Assessment, error) {
	return trustedSessionAssessment(t.Name(), toolpolicy.EffectWorkflow), nil
}

func trustedSessionAssessment(action string, effect toolpolicy.Effect) toolpolicy.Assessment {
	return toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorFastAllow,
		Action:   action,
		Effects:  []toolpolicy.Effect{effect},
		Scope:    toolpolicy.ScopeWorkspace,
		ReadOnly: effect == toolpolicy.EffectObserve,
		RiskHint: toolpolicy.RiskLow,
		Reason:   "trusted session tool",
	}
}

func humanOnlyAssessment(action, reason string) toolpolicy.Assessment {
	return toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorHumanOnly,
		Action:   action,
		Effects:  []toolpolicy.Effect{toolpolicy.EffectUnknown},
		Scope:    toolpolicy.ScopeUnknown,
		RiskHint: toolpolicy.RiskHigh,
		Reason:   reason,
	}
}

func firstPolicyValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
