package tools

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

func TestFileToolsAssessInvocationScopeAndEffects(t *testing.T) {
	workspace := t.TempDir()
	ctx := toolpolicy.Context{Workspace: workspace, CWD: workspace}
	tests := []struct {
		name      string
		tool      PermissionAssessor
		path      string
		behavior  toolpolicy.Behavior
		effect    toolpolicy.Effect
		readOnly  bool
		risk      toolpolicy.Risk
		wantScope toolpolicy.Scope
	}{
		{name: "workspace read", tool: NewReadFileTool(workspace), path: "notes.md", behavior: toolpolicy.BehaviorFastAllow, effect: toolpolicy.EffectObserve, readOnly: true, risk: toolpolicy.RiskLow, wantScope: toolpolicy.ScopeWorkspace},
		{name: "external read", tool: NewReadFileTool(workspace), path: filepath.Join(filepath.Dir(workspace), "external.md"), behavior: toolpolicy.BehaviorReviewable, effect: toolpolicy.EffectObserve, readOnly: true, risk: toolpolicy.RiskHigh, wantScope: toolpolicy.ScopeExternal},
		{name: "workspace write", tool: NewWriteFileTool(workspace), path: "notes.md", behavior: toolpolicy.BehaviorFastAllow, effect: toolpolicy.EffectMutate, risk: toolpolicy.RiskMedium, wantScope: toolpolicy.ScopeWorkspace},
		{name: "workspace edit", tool: NewEditFileTool(workspace), path: "notes.md", behavior: toolpolicy.BehaviorFastAllow, effect: toolpolicy.EffectMutate, risk: toolpolicy.RiskMedium, wantScope: toolpolicy.ScopeWorkspace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment, err := tt.tool.AssessPermission(ctx, json.RawMessage(`{"path":`+quotedJSON(tt.path)+`}`))
			if err != nil {
				t.Fatalf("AssessPermission() error = %v", err)
			}
			if assessment.Behavior != tt.behavior || assessment.Scope != tt.wantScope || assessment.ReadOnly != tt.readOnly || assessment.RiskHint != tt.risk {
				t.Fatalf("assessment = %+v", assessment)
			}
			if len(assessment.Effects) != 1 || assessment.Effects[0] != tt.effect {
				t.Fatalf("effects = %v, want [%s]", assessment.Effects, tt.effect)
			}
		})
	}
}

func TestBashToolAssessesReadOnlyAndMutatingCommands(t *testing.T) {
	workspace := t.TempDir()
	tool := NewBashTool(workspace)
	ctx := toolpolicy.Context{Workspace: workspace, CWD: workspace}
	tests := []struct {
		name     string
		args     string
		behavior toolpolicy.Behavior
		readOnly bool
		risk     toolpolicy.Risk
	}{
		{name: "read only", args: `{"command":"git status --short"}`, behavior: toolpolicy.BehaviorFastAllow, readOnly: true, risk: toolpolicy.RiskLow},
		{name: "mutation", args: `{"command":"go test ./..."}`, behavior: toolpolicy.BehaviorReviewable, risk: toolpolicy.RiskMedium},
		{name: "critical", args: `{"command":"rm -rf build"}`, behavior: toolpolicy.BehaviorReviewable, risk: toolpolicy.RiskCritical},
		{name: "invalid arguments", args: `{`, behavior: toolpolicy.BehaviorHumanOnly, risk: toolpolicy.RiskHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment, err := tool.AssessPermission(ctx, json.RawMessage(tt.args))
			if err != nil {
				t.Fatalf("AssessPermission() error = %v", err)
			}
			if assessment.Behavior != tt.behavior || assessment.ReadOnly != tt.readOnly || assessment.RiskHint != tt.risk {
				t.Fatalf("assessment = %+v", assessment)
			}
		})
	}
}

func TestSessionToolDeclaresDeterministicFastAllow(t *testing.T) {
	tool := NewAskUserQuestionTool(nil)
	assessment, err := tool.AssessPermission(toolpolicy.Context{}, nil)
	if err != nil {
		t.Fatalf("AssessPermission() error = %v", err)
	}
	if assessment.Behavior != toolpolicy.BehaviorFastAllow || assessment.RiskHint != toolpolicy.RiskLow {
		t.Fatalf("assessment = %+v", assessment)
	}
}

func quotedJSON(value string) string {
	b, _ := json.Marshal(value)
	return string(b)
}
