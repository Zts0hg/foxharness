package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/toolpolicy"
	"github.com/Zts0hg/foxharness/internal/tools"
)

func TestClassifyAllowsWorkspaceFileToolsAndReviewsEscapes(t *testing.T) {
	workspace := t.TempDir()
	call := toolCall("write_file", map[string]string{"path": "internal/new.go"})
	assessment, err := tools.NewWriteFileTool(workspace).AssessPermission(toolpolicy.Context{Workspace: workspace, CWD: workspace}, call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	got := Classify(workspace, workspace, SourceMain, call, assessment)
	if !got.AllowFastPath || got.Request.Risk != RiskMedium {
		t.Fatalf("workspace write classification = %+v, want fast medium", got)
	}

	escape := toolCall("read_file", map[string]string{"path": "../secret.txt"})
	assessment, err = tools.NewReadFileTool(workspace).AssessPermission(toolpolicy.Context{Workspace: workspace, CWD: workspace}, escape.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	got = Classify(workspace, workspace, SourceMain, escape, assessment)
	if !got.RequiresReview || got.Request.Risk != RiskHigh {
		t.Fatalf("escape classification = %+v, want review high", got)
	}
}

func TestClassifyReviewsWorkspaceSymlinkEscapes(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "external")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	call := toolCall("write_file", map[string]string{"path": "external/new.txt"})
	assessment, err := tools.NewWriteFileTool(workspace).AssessPermission(toolpolicy.Context{Workspace: workspace, CWD: workspace}, call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	got := Classify(workspace, workspace, SourceMain, call, assessment)
	if !got.RequiresReview || got.Request.Risk != RiskHigh {
		t.Fatalf("symlink escape classification = %+v, want review high", got)
	}
}

func TestReadOnlyBashFastPathIsConservative(t *testing.T) {
	workspace := t.TempDir()
	sub := filepath.Join(workspace, "sub")
	if !IsReadOnlyBash("pwd && ls -la . | head -20", workspace, sub) {
		t.Fatal("read-only chain should be allowed")
	}
	if IsReadOnlyBash("cd && cat .ssh/id_rsa", workspace, workspace) {
		t.Fatal("cd should not be allowed in read-only fast path")
	}
	if IsReadOnlyBash("GIT_EXTERNAL_DIFF=/tmp/runme git diff", workspace, workspace) {
		t.Fatal("environment assignments should not be allowed in read-only fast path")
	}
	if IsReadOnlyBash("sed -n 1p go.mod", workspace, workspace) {
		t.Fatal("sed should not be allowed in read-only fast path")
	}
	if IsReadOnlyBash("sed -i s/a/b/g file.txt", workspace, workspace) {
		t.Fatal("sed -i should not be allowed")
	}
	if IsReadOnlyBash("cat ../secret.txt", workspace, workspace) {
		t.Fatal("workspace escape should not be allowed")
	}
	if IsReadOnlyBash("cat ~/.ssh/id_rsa", workspace, workspace) {
		t.Fatal("tilde-expanded home path should not be allowed")
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret"), filepath.Join(workspace, "secret")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if IsReadOnlyBash("cat secret", workspace, workspace) {
		t.Fatal("bare symlink path should not be allowed")
	}
	if IsReadOnlyBash("echo hi > file.txt", workspace, workspace) {
		t.Fatal("redirect should not be allowed")
	}
	if IsReadOnlyBash("git reset --hard HEAD", workspace, workspace) {
		t.Fatal("git reset should not be allowed")
	}
	if IsReadOnlyBash("git diff --output=/tmp/out", workspace, workspace) {
		t.Fatal("git diff --output should not be allowed")
	}
	if IsReadOnlyBash("git branch new-work", workspace, workspace) {
		t.Fatal("git branch mutation should not be allowed")
	}
	if IsReadOnlyBash("git remote set-url origin git@example.com:x/y", workspace, workspace) {
		t.Fatal("git remote mutation should not be allowed")
	}
	if IsReadOnlyBash("rg --pre=rm pattern .", workspace, workspace) {
		t.Fatal("rg --pre should not be allowed")
	}
	if IsReadOnlyBash("find . -delete", workspace, workspace) {
		t.Fatal("find -delete should not be allowed")
	}
	if IsReadOnlyBash("find -L . -maxdepth 1", workspace, workspace) {
		t.Fatal("find -L should not be allowed")
	}
	if IsReadOnlyBash("find -H . -maxdepth 1", workspace, workspace) {
		t.Fatal("find -H should not be allowed")
	}
	if !IsReadOnlyBash("git status --short && git diff -- go.mod", workspace, workspace) {
		t.Fatal("read-only git status/diff should be allowed")
	}
}

func TestClassifyReviewsCompositeAndUnknownTools(t *testing.T) {
	for _, name := range []string{"delegate_task", "skill", "custom_tool"} {
		assessment := toolpolicy.Assessment{
			Behavior: toolpolicy.BehaviorReviewable, Action: name + " exact invocation",
			Effects: []toolpolicy.Effect{toolpolicy.EffectWorkflow}, Scope: toolpolicy.ScopeWorkspace,
			RiskHint: toolpolicy.RiskMedium, Reason: "declared reviewable capability",
		}
		got := Classify(t.TempDir(), "", SourceMain, toolCall(name, map[string]string{}), assessment)
		if !got.RequiresReview {
			t.Fatalf("%s classification = %+v, want review", name, got)
		}
	}
	got := Classify(t.TempDir(), "", SourceMain, toolCall("missing_metadata", map[string]string{"target": "external"}))
	if !got.HumanOnly || got.RequiresReview {
		t.Fatalf("missing metadata classification = %+v, want human only", got)
	}
	if !strings.Contains(got.Request.Action, `"target":"external"`) {
		t.Fatalf("missing metadata action = %q, want exact normalized arguments", got.Request.Action)
	}
}

func TestClassifyCompositeActionIncludesArguments(t *testing.T) {
	assessment := toolpolicy.Assessment{
		Behavior: toolpolicy.BehaviorReviewable, Action: "skill review pr-9",
		Effects: []toolpolicy.Effect{toolpolicy.EffectWorkflow}, Scope: toolpolicy.ScopeWorkspace, RiskHint: RiskLow,
	}
	got := Classify(t.TempDir(), "", SourceMain, toolCall("skill", map[string]string{"name": "review", "arguments": "pr-9"}), assessment)
	if got.Request.Action != "skill review pr-9" {
		t.Fatalf("composite action = %q, want assessor action", got.Request.Action)
	}
}

func TestClassifyRejectsContradictoryCapabilityAssessments(t *testing.T) {
	tests := []struct {
		name       string
		assessment toolpolicy.Assessment
	}{
		{name: "missing effects", assessment: toolpolicy.Assessment{Behavior: toolpolicy.BehaviorFastAllow, Action: "observe", Scope: toolpolicy.ScopeWorkspace, RiskHint: RiskLow}},
		{name: "unknown scope", assessment: toolpolicy.Assessment{Behavior: toolpolicy.BehaviorReviewable, Action: "observe", Effects: []toolpolicy.Effect{toolpolicy.EffectObserve}, RiskHint: RiskLow}},
		{name: "external fast allow", assessment: toolpolicy.Assessment{Behavior: toolpolicy.BehaviorFastAllow, Action: "read external", Effects: []toolpolicy.Effect{toolpolicy.EffectObserve}, Scope: toolpolicy.ScopeExternal, RiskHint: RiskLow}},
		{name: "delegate without nested enforcement", assessment: toolpolicy.Assessment{Behavior: toolpolicy.BehaviorReviewable, Action: "delegate", Effects: []toolpolicy.Effect{toolpolicy.EffectDelegate}, Scope: toolpolicy.ScopeWorkspace, RiskHint: RiskLow}},
		{name: "execute without command", assessment: toolpolicy.Assessment{Behavior: toolpolicy.BehaviorReviewable, Action: "execute", Effects: []toolpolicy.Effect{toolpolicy.EffectExecute}, Scope: toolpolicy.ScopeWorkspace, RiskHint: RiskMedium}},
		{name: "read only mutation", assessment: toolpolicy.Assessment{Behavior: toolpolicy.BehaviorReviewable, Action: "mutate", Effects: []toolpolicy.Effect{toolpolicy.EffectMutate}, Scope: toolpolicy.ScopeWorkspace, ReadOnly: true, RiskHint: RiskMedium}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(t.TempDir(), "", SourceMain, toolCall("test", map[string]string{}), tt.assessment)
			if !got.HumanOnly || got.AllowFastPath || got.RequiresReview {
				t.Fatalf("classification = %+v, want human only", got)
			}
		})
	}
}

func TestBashRiskUsesShellTokens(t *testing.T) {
	workspace := t.TempDir()
	tool := tools.NewBashTool(workspace)
	call := toolCall("bash", map[string]string{"command": "rm\t-rf dir"})
	assessment, err := tool.AssessPermission(toolpolicy.Context{Workspace: workspace, CWD: workspace}, call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	got := Classify(workspace, workspace, SourceMain, call, assessment)
	if got.Request.Risk != RiskCritical {
		t.Fatalf("rm tab risk = %q, want critical", got.Request.Risk)
	}
	call = toolCall("bash", map[string]string{"command": "git\tpush origin main"})
	assessment, err = tool.AssessPermission(toolpolicy.Context{Workspace: workspace, CWD: workspace}, call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	got = Classify(workspace, workspace, SourceMain, call, assessment)
	if got.Request.Risk != RiskHigh {
		t.Fatalf("git push tab risk = %q, want high", got.Request.Risk)
	}
}

func toolCall(name string, args map[string]string) schema.ToolCall {
	raw, _ := json.Marshal(args)
	return schema.ToolCall{ID: "call-1", Name: name, Arguments: raw}
}
