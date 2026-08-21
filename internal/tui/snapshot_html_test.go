package tui

import (
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/app"
)

func TestRenderSceneHTMLIsSelfContained(t *testing.T) {
	html, err := RenderSceneHTML("transcript", 110, 32)
	if err != nil {
		t.Fatalf("RenderSceneHTML error = %v", err)
	}
	for _, want := range []string{
		"<!doctype html>",
		"@font-face",
		"data:font/woff2;base64,", // font embedded, no external reference
		"background:" + SnapshotBackground,
		"fix the sidebar layout bug", // scene content survives conversion
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("HTML missing %q", want)
		}
	}
	if strings.Contains(html, "\x1b[") {
		t.Fatal("HTML still contains raw ANSI escape sequences")
	}
	if strings.Contains(html, "url(\"http") || strings.Contains(html, "url('http") || strings.Contains(html, "file://") {
		t.Fatal("HTML references an external resource; it must be self-contained")
	}
}

func TestRenderSceneHTMLUnknownSceneErrors(t *testing.T) {
	if _, err := RenderSceneHTML("nope", 110, 32); err == nil {
		t.Fatal("expected error for unknown scene, got nil")
	}
}

func TestRenderSessionHTMLRendersRecords(t *testing.T) {
	records := []app.ConversationRecord{
		{Sequence: 1, Role: "user", Content: "你好，渲染真实会话"},
		{Sequence: 2, Role: "assistant", Content: "Rendering the real session now."},
	}
	html := RenderSessionHTML(records, 110, 32)
	for _, want := range []string{
		"@font-face",
		"你好，渲染真实会话",
		"Rendering the real session now.",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("session HTML missing %q", want)
		}
	}
}

func TestAnsiToSpansMapsTruecolorToCSS(t *testing.T) {
	out := ansiToSpans("\x1b[38;2;120;220;232mcyan\x1b[0m plain \x1b[1;48;2;45;42;46mbg\x1b[0m")
	if !strings.Contains(out, "color:rgb(120,220,232)") {
		t.Fatalf("missing foreground CSS: %s", out)
	}
	if !strings.Contains(out, "background:rgb(45,42,46)") {
		t.Fatalf("missing background CSS: %s", out)
	}
	if !strings.Contains(out, "font-weight:700") {
		t.Fatalf("missing bold CSS: %s", out)
	}
}

func TestAnsiToSpansEscapesHTML(t *testing.T) {
	out := ansiToSpans("a <b> & c")
	if strings.Contains(out, "<b>") || !strings.Contains(out, "&lt;b&gt;") || !strings.Contains(out, "&amp;") {
		t.Fatalf("HTML metacharacters not escaped: %s", out)
	}
}
