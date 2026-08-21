package feishu

import "testing"

func TestParseSessionDirective(t *testing.T) {
	for _, test := range []struct {
		name     string
		input    string
		wantNew  bool
		wantText string
	}{
		{name: "plain", input: "检查日志", wantText: "检查日志"},
		{name: "slash new with prompt", input: "/new 检查日志", wantNew: true, wantText: "检查日志"},
		{name: "slash new only", input: "/new", wantNew: true, wantText: "/new"},
		{name: "chinese new", input: "新会话 修复 bug", wantNew: true, wantText: "修复 bug"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gotNew, gotText := parseSessionDirective(test.input)
			if gotNew != test.wantNew || gotText != test.wantText {
				t.Fatalf("parseSessionDirective(%q) = %t/%q, want %t/%q", test.input, gotNew, gotText, test.wantNew, test.wantText)
			}
		})
	}
}
