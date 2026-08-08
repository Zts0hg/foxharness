package tui

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestSceneNamesReturnsOrderedCatalog(t *testing.T) {
	names := SceneNames()
	if len(names) == 0 {
		t.Fatal("SceneNames() returned no scenes")
	}
	want := []string{
		"transcript", "sidebar", "permission-form", "approval-form",
		"plan-form", "ask-form", "effort-form", "selector",
	}
	if len(names) != len(want) {
		t.Fatalf("SceneNames() = %v, want %v", names, want)
	}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("SceneNames()[%d] = %q, want %q", i, names[i], name)
		}
	}
}

func TestRenderSceneANSIUnknownSceneErrors(t *testing.T) {
	_, err := RenderSceneANSI("does-not-exist", 100, 30)
	if err == nil {
		t.Fatal("expected error for unknown scene, got nil")
	}
	for _, name := range SceneNames() {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error %q does not list available scene %q", err.Error(), name)
		}
	}
}

func TestRenderSceneANSIIsDeterministic(t *testing.T) {
	for _, name := range SceneNames() {
		first, err := RenderSceneANSI(name, 110, 32)
		if err != nil {
			t.Fatalf("RenderSceneANSI(%q) error = %v", name, err)
		}
		if strings.TrimSpace(stripANSI(first)) == "" {
			t.Fatalf("RenderSceneANSI(%q) produced empty output", name)
		}
		second, err := RenderSceneANSI(name, 110, 32)
		if err != nil {
			t.Fatalf("RenderSceneANSI(%q) second call error = %v", name, err)
		}
		if first != second {
			t.Fatalf("RenderSceneANSI(%q) not deterministic across calls", name)
		}
	}
}

func TestRenderSceneANSIPreservesStyling(t *testing.T) {
	// Guards against the offline renderer downgrading to a colorless (Ascii)
	// profile, which would strip the theme's styling from the captured frame
	// and defeat faithful reproduction (NFR-003).
	out, err := RenderSceneANSI("permission-form", 120, 34)
	if err != nil {
		t.Fatalf("RenderSceneANSI error = %v", err)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("rendered frame contains no ANSI styling; the color profile was stripped")
	}
}

func TestRecolorForImageMapsPaletteToTruecolor(t *testing.T) {
	// codex-theme frames use 16-color ANSI (e.g. cyan=36, magenta=35,
	// bright-black=90). RecolorForImage must rewrite these to the fixed Monokai
	// Pro truecolor values and leave no bare 16-color foreground codes.
	in := "\x1b[36mcyan\x1b[0m \x1b[1;35mmagenta\x1b[0m \x1b[90mgray\x1b[0m"
	out := RecolorForImage(in)
	for _, want := range []string{
		"38;2;120;220;232", // cyan  #78dce8
		"38;2;171;157;242", // magenta #ab9df2
		"38;2;114;112;114", // gray  #727072
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("RecolorForImage output missing truecolor %q:\n%q", want, out)
		}
	}
	for _, seq := range regexp.MustCompile(`\x1b\[([0-9;]*)m`).FindAllStringSubmatch(out, -1) {
		for _, p := range strings.Split(seq[1], ";") {
			n, err := strconv.Atoi(p)
			if err != nil {
				continue
			}
			if (n >= 30 && n <= 37) || (n >= 90 && n <= 97) || (n >= 40 && n <= 47) || (n >= 100 && n <= 107) {
				t.Fatalf("RecolorForImage left a bare 16-color code %d in %q", n, seq[0])
			}
		}
	}
}

func TestRenderSceneANSISceneMarkers(t *testing.T) {
	cases := map[string]string{
		"transcript":      "fix the sidebar layout bug",
		"sidebar":         "MEMORY",
		"permission-form": "Permissions",
		"approval-form":   "Approve tool call",
		"plan-form":       "Refactor sidebar layout",
		"ask-form":        "Which rendering approach",
		"effort-form":     "Effort",
		"selector":        "(current)",
	}
	for name, marker := range cases {
		out, err := RenderSceneANSI(name, 120, 34)
		if err != nil {
			t.Fatalf("RenderSceneANSI(%q) error = %v", name, err)
		}
		if !strings.Contains(stripANSI(out), marker) {
			t.Fatalf("scene %q output missing marker %q:\n%s", name, marker, stripANSI(out))
		}
	}
}
