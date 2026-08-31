package subagent

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

func TestDelegateTaskPermissionAssessmentUsesExecutionDefaults(t *testing.T) {
	tool := NewTool(&recordingRunner{enforce: true}, "parent")
	tests := []struct {
		name     string
		args     string
		readOnly bool
		risk     toolpolicy.Risk
	}{
		{name: "omitted read only defaults true", args: `{"task":"search project docs"}`, readOnly: true, risk: toolpolicy.RiskLow},
		{name: "explicit read only", args: `{"task":"search project docs","read_only":true}`, readOnly: true, risk: toolpolicy.RiskLow},
		{name: "writable", args: `{"task":"change project files","read_only":false}`, readOnly: false, risk: toolpolicy.RiskMedium},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assessment, err := tool.AssessPermission(toolpolicy.Context{}, json.RawMessage(tt.args))
			if err != nil {
				t.Fatalf("AssessPermission() error = %v", err)
			}
			if assessment.Behavior != toolpolicy.BehaviorReviewable || assessment.ReadOnly != tt.readOnly || assessment.RiskHint != tt.risk {
				t.Fatalf("assessment = %+v", assessment)
			}
			if !assessment.NestedEnforcement {
				t.Fatal("NestedEnforcement = false, want true")
			}
			if !strings.Contains(assessment.Action, fmt.Sprintf("read_only=%t", tt.readOnly)) {
				t.Fatalf("action = %q, want effective read_only value", assessment.Action)
			}
		})
	}
}

func TestDelegateTaskPermissionAssessmentFailsClosedForInvalidTask(t *testing.T) {
	tool := NewTool(&recordingRunner{}, "parent")
	assessment, err := tool.AssessPermission(toolpolicy.Context{}, json.RawMessage(`{"task":""}`))
	if err != nil {
		t.Fatalf("AssessPermission() error = %v", err)
	}
	if assessment.Behavior != toolpolicy.BehaviorHumanOnly {
		t.Fatalf("behavior = %q, want human_only", assessment.Behavior)
	}
}
