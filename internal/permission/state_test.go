package permission

import "testing"

func TestStateDefaultsUnrememberedFullAccessToAskEffectiveMode(t *testing.T) {
	state := NewState(ModeFullAccess, false)
	got := state.Snapshot()
	if got.SelectedMode != ModeFullAccess {
		t.Fatalf("SelectedMode = %q, want full access", got.SelectedMode)
	}
	if got.EffectiveMode != ModeAsk {
		t.Fatalf("EffectiveMode = %q, want ask", got.EffectiveMode)
	}
	if !got.FullAccessNeedsWarning {
		t.Fatal("FullAccessNeedsWarning = false, want true")
	}
}

func TestStateRetainsGrantsAcrossModeChangesAndClearsExplicitly(t *testing.T) {
	state := NewState(ModeAsk, false)
	request := Request{ToolName: "bash", Arguments: `{"command":"go test ./..."}`, CWD: "/work", Workspace: "/work", Source: SourceMain}
	state.AddGrant(GrantForRequest(request))

	state.SetSelected(ModeApprove, false)
	if _, ok := state.MatchingGrant(request); !ok {
		t.Fatal("grant missing after mode change")
	}
	if got := state.ClearGrants(); got != 1 {
		t.Fatalf("ClearGrants() = %d, want 1", got)
	}
	if _, ok := state.MatchingGrant(request); ok {
		t.Fatal("grant still present after clear")
	}
}

func TestNormalizeModeDefaultsUnknownToAsk(t *testing.T) {
	if got := NormalizeMode("read-only"); got != ModeAsk {
		t.Fatalf("NormalizeMode(read-only) = %q, want ask", got)
	}
}

func TestGrantKeyUsesFileMutationTargetScope(t *testing.T) {
	first := Request{ToolName: "write_file", ToolCall: toolCall("write_file", map[string]string{"path": "../outside.txt", "content": "one"}), CWD: "/work/project", Workspace: "/work/project", Source: SourceMain}
	second := Request{ToolName: "edit_file", ToolCall: toolCall("edit_file", map[string]string{"path": "../outside.txt", "old_string": "one", "new_string": "two"}), CWD: "/work/project", Workspace: "/work/project", Source: SourceMain}
	if GrantKeyFor(first) != GrantKeyFor(second) {
		t.Fatal("write/edit grants for the same canonical mutation target should match")
	}
}

func TestGrantKeyCanonicalizesBashWhitespace(t *testing.T) {
	first := Request{ToolName: "bash", ToolCall: toolCall("bash", map[string]string{"command": "git\tpush origin main"}), CWD: "/work/project", Workspace: "/work/project", Source: SourceMain}
	second := Request{ToolName: "bash", ToolCall: toolCall("bash", map[string]string{"command": "git push origin main"}), CWD: "/work/project", Workspace: "/work/project", Source: SourceMain}
	if GrantKeyFor(first) != GrantKeyFor(second) {
		t.Fatal("equivalent bash command whitespace should share a grant")
	}
}

func TestGrantKeyPreservesBashStructure(t *testing.T) {
	first := Request{ToolName: "bash", ToolCall: toolCall("bash", map[string]string{"command": "git push origin main && git status"}), CWD: "/work/project", Workspace: "/work/project", Source: SourceMain}
	second := Request{ToolName: "bash", ToolCall: toolCall("bash", map[string]string{"command": "git push origin main git status"}), CWD: "/work/project", Workspace: "/work/project", Source: SourceMain}
	if GrantKeyFor(first) == GrantKeyFor(second) {
		t.Fatal("different bash command structures should not share a grant")
	}
}
