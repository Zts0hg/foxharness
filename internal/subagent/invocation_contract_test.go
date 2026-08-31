package subagent

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/toolpolicy"
)

func TestIACHD001DelegateSchemaAndInputDomainRemainNarrow(t *testing.T) {
	runner := &recordingRunner{enforce: true, result: &Result{Status: OutcomeSucceeded}}
	tool := NewTool(runner, "parent")
	definition := tool.Definition()
	if definition.Name != "delegate_task" || !strings.Contains(definition.Description, "Subagent") {
		t.Fatalf("delegate definition identity = %#v", definition)
	}
	schemaMap, ok := definition.InputSchema.(map[string]interface{})
	if !ok || schemaMap["type"] != "object" {
		t.Fatalf("delegate schema = %#v", definition.InputSchema)
	}
	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok || len(properties) != 2 {
		t.Fatalf("delegate properties = %#v", schemaMap["properties"])
	}
	if properties["task"].(map[string]interface{})["type"] != "string" || properties["read_only"].(map[string]interface{})["type"] != "boolean" {
		t.Fatalf("delegate property types = %#v", properties)
	}
	if required := schemaMap["required"].([]string); !reflect.DeepEqual(required, []string{"task"}) {
		t.Fatalf("delegate required fields = %v", required)
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{}`),
		json.RawMessage(`{"task":false}`),
		json.RawMessage(`{"task":"   "}`),
		json.RawMessage(`{"task":"inspect","read_only":"yes"}`),
	} {
		if _, err := tool.Execute(context.Background(), raw); err == nil {
			t.Fatalf("delegate accepted invalid input %s", raw)
		}
	}
	if runner.calls != 0 {
		t.Fatalf("invalid inputs reached child runner %d times", runner.calls)
	}
}

func TestIACHD002AssessmentMatchesNormalizedExecution(t *testing.T) {
	for _, test := range []struct {
		name         string
		raw          json.RawMessage
		wantTask     string
		wantReadOnly bool
		wantRisk     toolpolicy.Risk
	}{
		{name: "default read only", raw: json.RawMessage(`{"task":"  inspect files  ","model":"other","depth":2}`), wantTask: "inspect files", wantReadOnly: true, wantRisk: toolpolicy.RiskLow},
		{name: "explicit writable", raw: json.RawMessage(`{"task":"  edit one file  ","read_only":false}`), wantTask: "edit one file", wantReadOnly: false, wantRisk: toolpolicy.RiskMedium},
	} {
		t.Run(test.name, func(t *testing.T) {
			runner := &recordingRunner{enforce: true, result: &Result{SessionID: "child", Report: "done", Status: OutcomeSucceeded}}
			tool := NewTool(runner, "parent")
			assessment, err := tool.AssessPermission(toolpolicy.Context{}, test.raw)
			if err != nil {
				t.Fatal(err)
			}
			if assessment.Behavior != toolpolicy.BehaviorReviewable || assessment.ReadOnly != test.wantReadOnly || assessment.RiskHint != test.wantRisk || !assessment.NestedEnforcement {
				t.Fatalf("delegate assessment = %#v", assessment)
			}
			if _, err := tool.Execute(context.Background(), test.raw); err != nil {
				t.Fatal(err)
			}
			if runner.calls != 1 || runner.request.Task != test.wantTask || runner.request.ReadOnly != test.wantReadOnly {
				t.Fatalf("normalized child request = %#v, calls %d", runner.request, runner.calls)
			}
		})
	}
}
