package tui

import (
	"context"
	"reflect"
	"testing"
)

func TestUITUI001InitialPromptPrefillsEditableInputAtCursorEnd(t *testing.T) {
	prompt := "inspect main.go\nthen summarize"
	m := NewModel(context.Background(), newFakeRunner(), Config{InitialPrompt: prompt})
	if got := string(m.input); got != prompt {
		t.Fatalf("initial input = %q, want %q", got, prompt)
	}
	if m.inputCursor != len([]rune(prompt)) {
		t.Fatalf("initial cursor = %d, want %d", m.inputCursor, len([]rune(prompt)))
	}

	m, _ = update(t, m, keyRunes(" now"))
	if got := string(m.input); got != prompt+" now" {
		t.Fatalf("edited initial input = %q, want %q", got, prompt+" now")
	}
}

func TestUITUI003BuiltinCatalogCancellationAndExitAliases(t *testing.T) {
	gotNames := make([]string, 0, len(slashCommands))
	for _, command := range slashCommands {
		gotNames = append(gotNames, command.Name)
	}
	wantNames := []string{
		"/status", "/session", "/clear", "/rewind", "/checkpoint", "/new",
		"/plan", "/model", "/theme", "/statusline", "/permissions", "/effort",
		"/compact", "/autodev", "/cancel", "/sidebar", "/help", "/exit",
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("built-in command catalog = %#v, want %#v", gotNames, wantNames)
	}

	idle := NewModel(context.Background(), newFakeRunner(), Config{})
	next, cmd := idle.handleSlashCommand("/cancel")
	idle = next.(Model)
	if cmd != nil || idle.status != "No active run" {
		t.Fatalf("idle /cancel = cmd %v status %q", cmd, idle.status)
	}

	cancelled := false
	active := NewModel(context.Background(), newFakeRunner(), Config{})
	active.running = true
	active.cancelRun = func() { cancelled = true }
	next, cmd = active.handleSlashCommand("/cancel")
	active = next.(Model)
	if cmd != nil || !cancelled || active.status != "Cancel requested" || !entriesContain(active.entries, "command", "Current run cancellation requested.") {
		t.Fatalf("active /cancel = cmd %v cancelled %v status %q entries %#v", cmd, cancelled, active.status, active.entries)
	}

	for _, command := range []string{"/exit", "/quit"} {
		next, cmd = idle.handleSlashCommand(command)
		if _, ok := next.(Model); !ok {
			t.Fatalf("%s returned %T, want Model", command, next)
		}
		assertQuitCommand(t, cmd)
	}
}
