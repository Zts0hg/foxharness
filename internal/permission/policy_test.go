package permission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Zts0hg/foxharness/internal/schema"
)

func TestClassifyAllowsWorkspaceFileToolsAndReviewsEscapes(t *testing.T) {
	workspace := t.TempDir()
	call := toolCall("write_file", map[string]string{"path": "internal/new.go"})
	got := Classify(workspace, workspace, SourceMain, call)
	if !got.AllowFastPath || got.Request.Risk != RiskMedium {
		t.Fatalf("workspace write classification = %+v, want fast medium", got)
	}

	escape := toolCall("read_file", map[string]string{"path": "../secret.txt"})
	got = Classify(workspace, workspace, SourceMain, escape)
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
	got := Classify(workspace, workspace, SourceMain, call)
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
	if IsReadOnlyBash("sed -i s/a/b/g file.txt", workspace, workspace) {
		t.Fatal("sed -i should not be allowed")
	}
	if IsReadOnlyBash("cat ../secret.txt", workspace, workspace) {
		t.Fatal("workspace escape should not be allowed")
	}
	if IsReadOnlyBash("cat ~/.ssh/id_rsa", workspace, workspace) {
		t.Fatal("tilde-expanded home path should not be allowed")
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
	if IsReadOnlyBash("find . -delete", workspace, workspace) {
		t.Fatal("find -delete should not be allowed")
	}
	if !IsReadOnlyBash("git status --short && git diff -- go.mod", workspace, workspace) {
		t.Fatal("read-only git status/diff should be allowed")
	}
}

func TestClassifyReviewsCompositeAndUnknownTools(t *testing.T) {
	for _, name := range []string{"delegate_task", "skill", "custom_tool"} {
		got := Classify(t.TempDir(), "", SourceMain, toolCall(name, map[string]string{}))
		if !got.RequiresReview {
			t.Fatalf("%s classification = %+v, want review", name, got)
		}
	}
}

func toolCall(name string, args map[string]string) schema.ToolCall {
	raw, _ := json.Marshal(args)
	return schema.ToolCall{ID: "call-1", Name: name, Arguments: raw}
}
