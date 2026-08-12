package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tui/selector"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPFTUI006NewSessionClearsSessionStateAndPreservesProcessSettings(t *testing.T) {
	runner := &projectHistoryRunner{fakeRunner: newFakeRunner(), projectHistory: []string{"prior project prompt"}}
	runner.collaborationMode = collaboration.ModeFormalPlan
	runner.permissionState = permission.NewState(permission.ModeApprove, false)
	runner.permissionState.AddGrant(permission.GrantForRequest(permission.Request{
		ToolName: "bash", CWD: runner.workDir, Workspace: runner.workDir, Source: permission.SourceMain,
	}))
	m := NewModel(context.Background(), runner, Config{ProviderProtocol: "openai", EffortOverride: "high"})
	m.queuedPrompts = testQueuedPrompts("must be discarded")
	m.entries = []entry{{role: "user", body: "old transcript"}}
	m.sidebarVisible = false

	m, _ = update(t, m, keyRunes("/new"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("/new command is nil")
	}
	m, _ = update(t, m, cmd())

	if m.sessionID != "sess-new" || runner.SessionID() != "sess-new" {
		t.Fatalf("new session identity = %q/%q", m.sessionID, runner.SessionID())
	}
	if entriesContain(m.entries, "user", "old transcript") || len(m.queuedPrompts) != 0 {
		t.Fatalf("new session retained transcript or queue: entries=%#v queue=%#v", m.entries, m.queuedPrompts)
	}
	if m.collaborationMode != collaboration.ModeDefault || runner.CollaborationMode() != collaboration.ModeDefault {
		t.Fatalf("new session retained Formal Plan: model=%q runner=%q", m.collaborationMode, runner.CollaborationMode())
	}
	if got := runner.PermissionSnapshot(); got.SelectedMode != permission.ModeApprove || got.SessionGrantCount != 0 {
		t.Fatalf("new session permission snapshot = %#v, want selected approve and no grants", got)
	}
	if m.modelName != "fake-model" || m.effortValue != "high" || m.providerProtocol != "openai" || m.sidebarVisible {
		t.Fatalf("process settings changed: model=%q effort=%q protocol=%q sidebar=%v", m.modelName, m.effortValue, m.providerProtocol, m.sidebarVisible)
	}
	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "prior project prompt" {
		t.Fatalf("project input history after /new = %q", got)
	}

	m.running = true
	m.input = nil
	m.inputCursor = 0
	before := runner.SessionID()
	m, _ = update(t, m, keyRunes("/clear"))
	m, blocked := update(t, m, keyEnter())
	if blocked != nil || runner.SessionID() != before || m.status != "Cannot create a new session while a run is active" {
		t.Fatalf("active /clear = cmd=%v session=%q status=%q", blocked, runner.SessionID(), m.status)
	}
}

func TestPFTUI012PermissionDecisionsAndFailedPersistenceAreAtomic(t *testing.T) {
	decisions := []struct {
		name     string
		action   int
		feedback string
		wantKind permission.UserDecisionKind
	}{
		{name: "allow once", action: 0, wantKind: permission.UserAllowOnce},
		{name: "allow session", action: 1, wantKind: permission.UserAllowSession},
		{name: "deny", action: 2, wantKind: permission.UserDeny},
		{name: "deny with feedback", action: 3, feedback: "use read_file", wantKind: permission.UserDenyFeedback},
	}
	for _, tc := range decisions {
		t.Run(tc.name, func(t *testing.T) {
			reply := make(chan permission.UserDecision, 1)
			m := NewModel(context.Background(), newFakeRunner(), Config{})
			m.approvalForm = &approvalForm{req: permissionRequest{
				approval: permission.ApprovalRequest{Request: permission.Request{Action: "bash mutate"}},
				reply:    reply,
			}, action: tc.action, feedback: []rune(tc.feedback)}
			next, _ := m.handleApprovalDone()
			m = next.(Model)
			decision := <-reply
			if decision.Kind != tc.wantKind || decision.Feedback != tc.feedback || m.approvalForm != nil {
				t.Fatalf("decision = %#v overlay=%#v", decision, m.approvalForm)
			}
		})
	}

	homeFile := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(homeFile, []byte("occupied"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := newFakeRunner()
	before := runner.PermissionSnapshot()
	m := NewModel(context.Background(), runner, Config{HomeDir: homeFile})
	next, _ := m.handleSlashCommand("/permissions approve")
	m = next.(Model)
	after := runner.PermissionSnapshot()
	if after != before || m.status != "Permissions save failed" || !entriesContainTitle(m.entries, "error", "permissions save failed") {
		t.Fatalf("failed persistence partially activated mode: before=%#v after=%#v status=%q entries=%#v", before, after, m.status, m.entries)
	}

	m = NewModel(context.Background(), runner, Config{HomeDir: homeFile})
	next, _ = m.handleSlashCommand("/permissions full-access")
	m = next.(Model)
	if m.permissionForm == nil {
		t.Fatal("full access warning did not open")
	}
	m, cmd := update(t, m, keyEsc())
	if cmd == nil {
		t.Fatal("cancelled warning did not return completion command")
	}
	m, _ = update(t, m, cmd())
	if m.permissionForm != nil || runner.PermissionSnapshot().EffectiveMode == permission.ModeFullAccess {
		t.Fatalf("cancelled warning activated mode: cmd=%v form=%#v snapshot=%#v", cmd, m.permissionForm, runner.PermissionSnapshot())
	}
}

func TestPFTUI014RewindActionsAndFailuresPreserveCurrentOrdering(t *testing.T) {
	newModel := func() (Model, *fakeRunner, *tuiCheckpointer) {
		runner := newFakeRunner()
		runner.history = []session.MessageRecord{
			historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "first"}),
			historyRecord(1, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "answer"}),
			historyRecord(2, "run-2", schema.Message{Role: schema.RoleUser, Content: "restore target"}),
			historyRecord(3, "run-2", schema.Message{Role: schema.RoleAssistant, Content: "future"}),
		}
		runner.restoreStateOK = true
		cp := &tuiCheckpointer{stats: &checkpoint.DiffStats{FilesChanged: 1}}
		runner.checkpointer = cp
		return NewModel(context.Background(), runner, Config{}), runner, cp
	}

	t.Run("restore both", func(t *testing.T) {
		m, runner, cp := newModel()
		m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreBoth, MessageID: "2"})
		if cp.rewind != "2" || runner.truncatedSeq != 2 || runner.restoreStateSeq != 2 || string(m.input) != "restore target" || m.status != "Rewind complete" {
			t.Fatalf("restore both = rewind=%q truncate=%d state=%d input=%q status=%q", cp.rewind, runner.truncatedSeq, runner.restoreStateSeq, m.input, m.status)
		}
		if entriesContain(m.entries, "assistant", "future") {
			t.Fatalf("restore both retained future transcript: %#v", m.entries)
		}
	})

	t.Run("conversation only", func(t *testing.T) {
		m, runner, cp := newModel()
		m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreConversation, MessageID: "2"})
		if cp.rewind != "" || runner.truncatedSeq != 2 || runner.restoreStateSeq != 2 || m.status != "Rewind complete" {
			t.Fatalf("conversation restore = rewind=%q truncate=%d state=%d status=%q", cp.rewind, runner.truncatedSeq, runner.restoreStateSeq, m.status)
		}
	})

	t.Run("code only", func(t *testing.T) {
		m, runner, cp := newModel()
		m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreCode, MessageID: "2"})
		if cp.rewind != "2" || runner.truncatedSeq != -1 || runner.restoreStateSeq != 0 || m.status != "Rewind complete" {
			t.Fatalf("code restore = rewind=%q truncate=%d state=%d status=%q", cp.rewind, runner.truncatedSeq, runner.restoreStateSeq, m.status)
		}
	})

	t.Run("cancel and no-op", func(t *testing.T) {
		for _, action := range []selector.RestoreAction{selector.ActionCancelled, selector.ActionNone} {
			m, runner, cp := newModel()
			m, _ = update(t, m, selector.ResultMsg{Action: action, MessageID: "2"})
			if cp.rewind != "" || runner.truncatedSeq != -1 || m.status != "Rewind cancelled" {
				t.Fatalf("action %v mutated rewind state", action)
			}
		}
	})

	t.Run("code failure stops code-only", func(t *testing.T) {
		runner := newFakeRunner()
		runner.history = []session.MessageRecord{historyRecord(2, "run", schema.Message{Role: schema.RoleUser, Content: "target"})}
		cp := &errorTuiCheckpointer{rewindErr: errors.New("code unavailable")}
		runner.checkpointer = cp
		m := NewModel(context.Background(), runner, Config{})
		m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreCode, MessageID: "2"})
		if runner.truncatedSeq != -1 || m.status != "Code restore failed" {
			t.Fatalf("code failure continued: truncate=%d status=%q", runner.truncatedSeq, m.status)
		}
	})

	t.Run("code failure in restore-both still restores conversation", func(t *testing.T) {
		runner := newFakeRunner()
		runner.history = []session.MessageRecord{
			historyRecord(2, "run", schema.Message{Role: schema.RoleUser, Content: "target"}),
			historyRecord(3, "run", schema.Message{Role: schema.RoleAssistant, Content: "future"}),
		}
		runner.restoreStateOK = true
		runner.checkpointer = &errorTuiCheckpointer{rewindErr: errors.New("code unavailable")}
		m := NewModel(context.Background(), runner, Config{})
		m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreBoth, MessageID: "2"})
		if runner.truncatedSeq != 2 || runner.restoreStateSeq != 2 || m.status != "Rewind complete" || string(m.input) != "target" {
			t.Fatalf("restore-both code failure ordering = truncate=%d state=%d status=%q input=%q", runner.truncatedSeq, runner.restoreStateSeq, m.status, m.input)
		}
	})

	t.Run("conversation failure prevents session-state restore", func(t *testing.T) {
		m, runner, _ := newModel()
		runner.truncateErr = errors.New("history unavailable")
		m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreConversation, MessageID: "2"})
		if runner.restoreStateSeq != 0 || m.status != "Conversation restore failed" {
			t.Fatalf("conversation failure ordering = state=%d status=%q", runner.restoreStateSeq, m.status)
		}
	})

	t.Run("session-state failure follows conversation restore", func(t *testing.T) {
		m, runner, _ := newModel()
		runner.restoreStateErr = errors.New("state unavailable")
		m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreConversation, MessageID: "2"})
		if runner.truncatedSeq != 2 || runner.restoreStateSeq != 2 || m.status != "Session state restore failed" {
			t.Fatalf("session-state failure ordering = truncate=%d state=%d status=%q", runner.truncatedSeq, runner.restoreStateSeq, m.status)
		}
	})
}

func TestPFTUI016ReporterMapsCanonicalFactsAndStreamingStateOnce(t *testing.T) {
	events := make(chan tea.Msg, 12)
	reporter := &channelReporter{events: events, operationID: 42}
	ctx := context.Background()
	reporter.OnRunStart(ctx, "session", "run")
	reporter.OnThinking(ctx, 1)
	reporter.OnCompaction(ctx, "session_history")
	reporter.OnToolCall(ctx, "bash", `{"command":"date"}`)
	reporter.OnToolResult(ctx, "bash", "today", false)
	reporter.OnMessageDelta(ctx, "hel")
	reporter.OnMessageDelta(ctx, "lo")
	reporter.OnMessage(ctx, "hello")
	reporter.OnRunError(ctx, "session", "run", errors.New("failed"))
	reporter.OnRunComplete(ctx, engine.RunResult{RunID: "run"})

	got := make([]runEventMsg, 0, 10)
	for i := 0; i < 10; i++ {
		got = append(got, (<-events).(runEventMsg))
	}
	wantStatuses := []string{
		"Run started: run",
		"Thinking turn 1",
		"Context compacted",
		"Calling tool: bash",
		"Tool complete: bash",
		"Assistant responding",
		"Assistant responding",
		"Assistant responded",
		"Run failed",
		"Run complete: run",
	}
	statuses := make([]string, len(got))
	for i, event := range got {
		statuses[i] = event.status
		if event.operationID != 42 {
			t.Fatalf("event %d operation ID = %d, want 42", i, event.operationID)
		}
	}
	if !reflect.DeepEqual(statuses, wantStatuses) {
		t.Fatalf("reporter statuses = %#v, want %#v", statuses, wantStatuses)
	}
	if !got[5].delta || !got[6].delta || !got[7].streamFinal || !got[8].err {
		t.Fatalf("reporter event flags = %#v", got)
	}

	m := NewModel(context.Background(), newFakeRunner(), Config{})
	m.entries = nil
	m.running = true
	m.activeOperationID = 42
	for _, event := range got[:8] {
		m.applyRunEvent(event)
	}
	if countEntriesContaining(m.entries, "assistant", "hello") != 1 {
		t.Fatalf("stream final was not represented once: %#v", m.entries)
	}
	m.running = false
	m.applyRunEvent(runEventMsg{operationID: 42, role: "assistant", body: "late", delta: true})
	if entriesContain(m.entries, "assistant", "late") {
		t.Fatalf("late delta mutated terminal transcript: %#v", m.entries)
	}
}

type errorTuiCheckpointer struct {
	rewindErr error
}

func (*errorTuiCheckpointer) TrackEdit(string, string) error    { return nil }
func (*errorTuiCheckpointer) MakeSnapshot(string) error         { return nil }
func (c *errorTuiCheckpointer) Rewind(string) ([]string, error) { return nil, c.rewindErr }
func (*errorTuiCheckpointer) GetDiffStats(string) (*checkpoint.DiffStats, error) {
	return &checkpoint.DiffStats{}, nil
}
func (*errorTuiCheckpointer) HasAnyChanges(string) (bool, error) { return true, nil }
func (*errorTuiCheckpointer) SetDisabled(bool)                   {}
func (*errorTuiCheckpointer) IsDisabled() bool                   { return false }
func (*errorTuiCheckpointer) RestoreStateFromLog() error         { return nil }

func entriesContainTitle(entries []entry, role string, text string) bool {
	for _, item := range entries {
		if item.role == role && item.title == text {
			return true
		}
	}
	return false
}
