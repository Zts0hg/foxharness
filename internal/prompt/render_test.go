package prompt

import "testing"

func TestM01RenderOrdersResolvedFragmentsExactly(t *testing.T) {
	fragments := []Fragment{
		Text("  base instructions  "),
		Section(" Project Instructions ", "  inspect before editing  "),
		Section("Session Working Memory", "line one\nline two"),
	}
	want := "base instructions\n\n## Project Instructions\n\ninspect before editing\n\n## Session Working Memory\n\nline one\nline two"

	if got := Render(fragments); got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
	if got := Render(fragments); got != want {
		t.Fatalf("second Render() = %q, want deterministic output %q", got, want)
	}
	if fragments[0].Body != "  base instructions  " || fragments[1].Title != " Project Instructions " {
		t.Fatalf("Render mutated resolved fragments: %#v", fragments)
	}
}

func TestM01RenderPreservesExplicitEmptySlotsAndSuppressesEmptySectionTitle(t *testing.T) {
	fragments := []Fragment{
		Text("first"),
		Text(""),
		Section("Empty Section", ""),
	}
	want := "first\n\n\n\n"
	if got := Render(fragments); got != want {
		t.Fatalf("Render() = %q, want %q", got, want)
	}
}
