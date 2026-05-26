package feishu

import (
	"testing"

	"github.com/Zts0hg/foxharness/internal/session"
)

func TestParseSessionDirective(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantNew  bool
		wantText string
	}{
		{name: "plain", input: "检查日志", wantText: "检查日志"},
		{name: "slash new with prompt", input: "/new 检查日志", wantNew: true, wantText: "检查日志"},
		{name: "slash new only", input: "/new", wantNew: true, wantText: "/new"},
		{name: "chinese new", input: "新会话 修复 bug", wantNew: true, wantText: "修复 bug"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotNew, gotText := parseSessionDirective(tt.input)
			if gotNew != tt.wantNew {
				t.Fatalf("forceNew = %v, want %v", gotNew, tt.wantNew)
			}
			if gotText != tt.wantText {
				t.Fatalf("text = %q, want %q", gotText, tt.wantText)
			}
		})
	}
}

func TestRunnerBuildRegistryIncludesTodoTools(t *testing.T) {
	runner := &Runner{workDir: t.TempDir()}
	sess := &session.Session{ID: "sess", RootDir: t.TempDir()}
	registry := runner.buildRegistry(sess, "chat")

	names := map[string]bool{}
	for _, def := range registry.GetAvailableTools() {
		names[def.Name] = true
	}
	for _, name := range []string{"read_todo", "update_todo"} {
		if !names[name] {
			t.Fatalf("registry missing %s", name)
		}
	}
}
