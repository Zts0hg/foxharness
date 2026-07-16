package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/settings"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tui/selector"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fakeRunner struct {
	sessionID  string
	sessionDir string
	workDir    string
	model      string

	runs              []string
	runModes          []collaboration.Mode
	runErr            error
	runErrs           []error
	setModelErr       error
	newErr            error
	nextRunID         int
	collaborationMode collaboration.Mode
	contextUsage      string
	history           []session.MessageRecord
	historyErr        error
	truncatedSeq      int64
	restoreStateSeq   int64
	restoreStateOK    bool
	restoreStateErr   error
	checkpointer      checkpoint.Checkpointer
	compactResult     *compaction.CompactResult
	compactErr        error
	compactInstr      string
	memoryIndex       string
	permissionState   *permission.State
}

// AutoMemoryIndex satisfies the Runner interface for the test double; tests that
// care about the sidebar Memory panel set memoryIndex explicitly.
func (r *fakeRunner) AutoMemoryIndex() string {
	return r.memoryIndex
}

type projectHistoryRunner struct {
	*fakeRunner
	projectHistory    []string
	projectHistoryErr error
}

func (r *projectHistoryRunner) ProjectInputHistory(limit int) ([]string, error) {
	if r.projectHistoryErr != nil {
		return nil, r.projectHistoryErr
	}
	if limit > 0 && len(r.projectHistory) > limit {
		return append([]string(nil), r.projectHistory[len(r.projectHistory)-limit:]...), nil
	}
	return append([]string(nil), r.projectHistory...), nil
}

func (r *fakeRunner) RunInCollaborationMode(ctx context.Context, prompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return r.runInCollaborationMode(ctx, prompt, mode, reporter)
}

func (r *fakeRunner) runInCollaborationMode(ctx context.Context, prompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	r.runs = append(r.runs, prompt)
	r.runModes = append(r.runModes, collaboration.Normalize(mode))
	r.nextRunID++
	runID := "run-1"
	if r.nextRunID > 1 {
		runID = "run-2"
	}
	reporter.OnRunStart(ctx, r.sessionID, runID)
	reporter.OnThinking(ctx, 1)
	reporter.OnToolCall(ctx, "bash", `{"command":"date"}`)
	reporter.OnToolResult(ctx, "bash", "2026年 5月17日 星期日 14时17分46秒 CST", false)
	runErr := r.runErr
	if len(r.runErrs) > 0 {
		runErr = r.runErrs[0]
		r.runErrs = r.runErrs[1:]
	}
	if runErr != nil {
		reporter.OnRunError(ctx, r.sessionID, runID, runErr)
		return nil, runErr
	}
	reporter.OnMessage(ctx, "answer: "+prompt)
	result := &engine.RunResult{
		FinalMessage: "answer: " + prompt,
		SessionID:    r.sessionID,
		RunID:        runID,
		MetricsPath:  "/tmp/metrics.jsonl",
		TracePath:    "/tmp/trace.jsonl",
	}
	reporter.OnRunComplete(ctx, *result)
	return result, nil
}

func (r *fakeRunner) NewSession(ctx context.Context) (string, error) {
	if r.newErr != nil {
		return "", r.newErr
	}
	r.sessionID = "sess-new"
	r.sessionDir = "/tmp/sess-new"
	r.collaborationMode = collaboration.ModeDefault
	return r.sessionID, nil
}

func (r *fakeRunner) SessionID() string {
	return r.sessionID
}

func (r *fakeRunner) SessionDir() string {
	return r.sessionDir
}

func (r *fakeRunner) WorkDir() string {
	return r.workDir
}

func (r *fakeRunner) Model() string {
	return r.model
}

func (r *fakeRunner) SetModel(model string) error {
	if r.setModelErr != nil {
		return r.setModelErr
	}
	r.model = model
	return nil
}

func (r *fakeRunner) ContextUsage() string {
	if r.contextUsage == "" {
		return "7%"
	}
	return r.contextUsage
}

func (r *fakeRunner) MessageHistory() ([]session.MessageRecord, error) {
	if r.historyErr != nil {
		return nil, r.historyErr
	}
	return append([]session.MessageRecord(nil), r.history...), nil
}

func (r *fakeRunner) TruncateMessageHistory(seq int64) error {
	r.truncatedSeq = seq
	var next []session.MessageRecord
	for _, record := range r.history {
		if record.Seq < seq {
			next = append(next, record)
		}
	}
	r.history = next
	return nil
}

func (r *fakeRunner) RestoreSessionStateBeforeMessage(seq int64) (bool, error) {
	r.restoreStateSeq = seq
	if r.restoreStateErr != nil {
		return false, r.restoreStateErr
	}
	return r.restoreStateOK, nil
}

func (r *fakeRunner) Checkpointer() checkpoint.Checkpointer {
	return r.checkpointer
}

func (r *fakeRunner) CollaborationMode() collaboration.Mode {
	return collaboration.Normalize(r.collaborationMode)
}

func (r *fakeRunner) SetCollaborationMode(mode collaboration.Mode) {
	r.collaborationMode = collaboration.Normalize(mode)
}

func (r *fakeRunner) CompactNow(ctx context.Context, customInstructions string) (*compaction.CompactResult, error) {
	r.compactInstr = customInstructions
	if r.compactErr != nil {
		return nil, r.compactErr
	}
	if r.compactResult != nil {
		return r.compactResult, nil
	}
	return &compaction.CompactResult{
		PreTokens:          1000,
		PostTokens:         200,
		MessagesSummarized: 15,
	}, nil
}

func (r *fakeRunner) PermissionSnapshot() permission.Snapshot {
	if r.permissionState == nil {
		r.permissionState = permission.NewState(permission.ModeAsk, false)
	}
	return r.permissionState.Snapshot()
}

func (r *fakeRunner) SetPermissionMode(mode permission.Mode, remembered bool) {
	if r.permissionState == nil {
		r.permissionState = permission.NewState(mode, remembered)
		return
	}
	r.permissionState.SetSelected(mode, remembered)
}

func (r *fakeRunner) ActivateFullAccess(remember bool) {
	if r.permissionState == nil {
		r.permissionState = permission.NewState(permission.ModeAsk, false)
	}
	r.permissionState.ActivateFullAccess(remember)
}

func (r *fakeRunner) ClearPermissionGrants() int {
	if r.permissionState == nil {
		return 0
	}
	return r.permissionState.ClearGrants()
}

type tuiCheckpointer struct {
	stats  *checkpoint.DiffStats
	rewind string
}

func (c *tuiCheckpointer) TrackEdit(filePath, messageID string) error { return nil }
func (c *tuiCheckpointer) MakeSnapshot(messageID string) error        { return nil }
func (c *tuiCheckpointer) Rewind(messageID string) ([]string, error) {
	c.rewind = messageID
	return []string{"main.go"}, nil
}
func (c *tuiCheckpointer) GetDiffStats(messageID string) (*checkpoint.DiffStats, error) {
	if c.stats == nil {
		return &checkpoint.DiffStats{}, nil
	}
	return c.stats, nil
}
func (c *tuiCheckpointer) HasAnyChanges(messageID string) (bool, error) { return false, nil }
func (c *tuiCheckpointer) SetDisabled(disabled bool)                    {}
func (c *tuiCheckpointer) IsDisabled() bool                             { return false }
func (c *tuiCheckpointer) RestoreStateFromLog() error                   { return nil }

func TestModelSubmitsPromptAndRendersRunEvents(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("inspect go.mod"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("submit command is nil")
	}
	if !m.running {
		t.Fatalf("model running = false, want true")
	}

	m, _ = update(t, m, cmd())
	if m.running {
		t.Fatalf("model running = true after completion")
	}
	if len(runner.runs) != 1 || runner.runs[0] != "inspect go.mod" {
		t.Fatalf("runs = %#v, want one submitted prompt", runner.runs)
	}
	if !entriesContain(m.entries, "user", "inspect go.mod") {
		t.Fatalf("entries missing user prompt: %#v", m.entries)
	}
	if !entriesContain(m.entries, "assistant", "answer: inspect go.mod") {
		t.Fatalf("entries missing assistant message: %#v", m.entries)
	}
	if !entriesContain(m.entries, "tool", "2026年 5月17日") {
		t.Fatalf("entries missing tool result: %#v", m.entries)
	}
	if entriesContain(m.entries, "system", "Session:") || entriesContain(m.entries, "system", "Run:") {
		t.Fatalf("run start details should stay out of transcript entries: %#v", m.entries)
	}
	if entriesContain(m.entries, "system", "Metrics:") || entriesContain(m.entries, "system", "Trace:") {
		t.Fatalf("run completion details should stay out of transcript entries: %#v", m.entries)
	}
	if !strings.Contains(m.status, "run-1") {
		t.Fatalf("status = %q, want completed run id", m.status)
	}
}

func TestBangInputRendering(t *testing.T) {
	runner := newFakeRunner()
	base := func() Model {
		m := NewModel(context.Background(), runner, Config{})
		m.width = 80
		m.height = 20
		return m
	}

	// Typing "!ls" renders as "! ls" (bang prompt + command, no duplicated '!'),
	// not the ordinary "> !ls".
	m, _ := update(t, base(), keyRunes("!ls"))
	rendered := stripANSI(m.renderInput(m.innerWidth()))
	if !strings.Contains(rendered, "! ls") {
		t.Fatalf("bang input should render as '! ls', got:\n%s", rendered)
	}
	if strings.Contains(rendered, "> !ls") {
		t.Fatalf("bang input must not keep the ordinary '> ' prompt, got:\n%s", rendered)
	}
	if rows := renderedInputContentRows(m); len(rows) == 0 || rows[0] != "ls" {
		t.Fatalf("rendered content rows = %#v, want first row \"ls\"", rows)
	}
	if _, col, ok := renderedInputCursorPosition(m); !ok || col != lipgloss.Width("! ls") {
		t.Fatalf("cursor col = %d (ok=%v), want %d (after \"! ls\")", col, ok, lipgloss.Width("! ls"))
	}

	// A lone "!" shows the bang prompt plus a shell-mode placeholder.
	m2, _ := update(t, base(), keyRunes("!"))
	rendered2 := stripANSI(m2.renderInput(m2.innerWidth()))
	if !strings.Contains(rendered2, "! ") || !strings.Contains(rendered2, "shell command") {
		t.Fatalf("empty bang should show '! ' + shell placeholder, got:\n%s", rendered2)
	}

	// Ordinary input still uses the "> " prompt.
	m3, _ := update(t, base(), keyRunes("hello"))
	rendered3 := stripANSI(m3.renderInput(m3.innerWidth()))
	if !strings.Contains(rendered3, "> hello") {
		t.Fatalf("ordinary input should keep '> ' prompt, got:\n%s", rendered3)
	}
}

func TestModelBangCommandRunsLocalShellWithoutModelRun(t *testing.T) {
	workDir := t.TempDir()
	runner := newFakeRunner()
	runner.workDir = workDir
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("!printf fox-bang"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("bang command did not dispatch shell command")
	}
	if !m.running {
		t.Fatalf("model running = false, want shell command running")
	}
	if got := string(m.input); got != "" {
		t.Fatalf("input after bang submit = %q, want empty", got)
	}

	m, _ = update(t, m, cmd())
	if m.running {
		t.Fatalf("model running = true after shell command completion")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("runner runs = %#v, want no model runs for bang command", runner.runs)
	}
	if !entriesContain(m.entries, "command", "fox-bang") {
		t.Fatalf("entries missing shell output: %#v", m.entries)
	}
	if len(m.inputHistory) == 0 || m.inputHistory[len(m.inputHistory)-1] != "!printf fox-bang" {
		t.Fatalf("input history = %#v, want bang command recorded", m.inputHistory)
	}
}

func TestModelBangCommandCompletionStartsQueuedPrompt(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("!printf shell"))
	m, shellCmd := update(t, m, keyEnter())
	if shellCmd == nil {
		t.Fatalf("bang command did not dispatch shell command")
	}

	m, cmd := update(t, m, keyRunes("queued prompt"))
	if cmd != nil {
		t.Fatalf("typing while shell command runs returned command")
	}
	m, cmd = update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("queueing prompt while shell command runs returned command")
	}
	if len(m.queuedPrompts) != 1 || m.queuedPrompts[0].text != "queued prompt" {
		t.Fatalf("queuedPrompts = %#v, want queued prompt", m.queuedPrompts)
	}

	m, queuedCmd := update(t, m, shellCommandFinishedMsg{
		command: "printf shell",
		result:  tools.BashCommandResult{Output: "shell"},
	})
	if queuedCmd == nil {
		t.Fatalf("shell command completion did not start queued prompt")
	}
	if !m.running {
		t.Fatalf("model running = false, want queued prompt running")
	}
	if len(m.queuedPrompts) != 0 {
		t.Fatalf("queuedPrompts = %#v, want empty after queued prompt starts", m.queuedPrompts)
	}
	if !entriesContain(m.entries, "user", "queued prompt") {
		t.Fatalf("entries missing queued prompt user message: %#v", m.entries)
	}

	m, _ = update(t, m, queuedCmd())
	if got := strings.Join(runner.runs, ","); got != "queued prompt" {
		t.Fatalf("runs = %#v, want queued prompt to run", runner.runs)
	}
}

func TestModelFailedBangCommandCompletionStartsQueuedPrompt(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("!false"))
	m, shellCmd := update(t, m, keyEnter())
	if shellCmd == nil {
		t.Fatalf("bang command did not dispatch shell command")
	}

	m, _ = update(t, m, keyRunes("queued after failure"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("queueing prompt while shell command runs returned command")
	}

	m, queuedCmd := update(t, m, shellCommandFinishedMsg{
		command: "false",
		result:  tools.BashCommandResult{Err: errors.New("exit status 1"), ExitCode: 1},
	})
	if queuedCmd == nil {
		t.Fatalf("failed shell command completion did not start queued prompt")
	}
	if !m.running {
		t.Fatalf("model running = false, want queued prompt running")
	}
	if !entriesContain(m.entries, "command", "exit status 1") {
		t.Fatalf("entries missing failed shell command output: %#v", m.entries)
	}

	m, _ = update(t, m, queuedCmd())
	if got := strings.Join(runner.runs, ","); got != "queued after failure" {
		t.Fatalf("runs = %#v, want queued prompt to run after shell failure", runner.runs)
	}
}

func TestModelBangCommandEmptyInputShowsHelp(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("!"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("empty bang returned command")
	}
	if len(runner.runs) != 0 {
		t.Fatalf("runner runs = %#v, want no model runs", runner.runs)
	}
	if !entriesContain(m.entries, "command", "Example: !ls") {
		t.Fatalf("entries missing bang help: %#v", m.entries)
	}
}

func TestModelBangCommandRejectedWhileRunActive(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true

	m, _ = update(t, m, keyRunes("!pwd"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("bang while running returned command")
	}
	if len(m.queuedPrompts) != 0 {
		t.Fatalf("queued prompts = %#v, want bang command not queued", m.queuedPrompts)
	}
	if string(m.input) != "!pwd" {
		t.Fatalf("input after rejected bang = %q, want preserved", string(m.input))
	}
	if !strings.Contains(m.status, "Shell command unavailable") {
		t.Fatalf("status = %q, want shell unavailable notice", m.status)
	}
}

func TestModelEscCancelsRunningBangCommand(t *testing.T) {
	workDir := t.TempDir()
	runner := newFakeRunner()
	runner.workDir = workDir
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("!sleep 2; printf done"))
	m, shellCmd := update(t, m, keyEnter())
	if shellCmd == nil {
		t.Fatalf("bang command did not dispatch shell command")
	}

	done := make(chan tea.Msg, 1)
	go func() {
		done <- shellCmd()
	}()

	m, escCmd := update(t, m, keyEsc())
	if escCmd == nil {
		t.Fatalf("esc did not schedule pending cancellation")
	}
	m, _ = update(t, m, escCmd())

	select {
	case msg := <-done:
		m, _ = update(t, m, msg)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("running bang command did not stop promptly after Esc cancellation")
	}

	if m.running {
		t.Fatalf("model running = true after cancelled bang command")
	}
	if len(m.entries) == 0 || m.entries[len(m.entries)-1].title != "Shell: !sleep 2; printf done" {
		t.Fatalf("entries missing cancelled shell command: %#v", m.entries)
	}
	if entriesContain(m.entries, "command", "done") {
		t.Fatalf("shell command continued after Esc cancellation: %#v", m.entries)
	}
}

func TestModelViewUsesCompactMessageRendering(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("hello, what's the day today?"))
	m, cmd := update(t, m, keyEnter())
	m, _ = update(t, m, cmd())

	view := m.View()
	plainView := stripANSI(view)
	for _, forbidden := range []string{
		"You ",
		"USER you",
		"SYSTEM run started",
		"TOOL call bash",
		"TOOL result bash",
		"SYSTEM thinking",
		"Planning turn",
		"Session: sess-1",
		"Run: run-1",
		"14:17",
	} {
		if strings.Contains(plainView, forbidden) {
			t.Fatalf("view contains verbose fragment %q:\n%s", forbidden, view)
		}
	}
	for _, want := range []string{
		"hello, what's the day today?",
		"⬢ Bash (date)",
		"└─ 2026年 5月17日",
		"answer: hello, what's the day today?",
	} {
		if !strings.Contains(plainView, want) {
			t.Fatalf("view missing compact fragment %q:\n%s", want, view)
		}
	}
}

func TestToolCallRenderingUsesReferenceStyleLabels(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "read", body: formatToolInvocation("read_file", `{"path":"internal/foo.go"}`), want: "⬢ Read (internal/foo.go)"},
		{name: "write", body: formatToolInvocation("write_file", `{"path":"cmd/app.go"}`), want: "⬢ Write (cmd/app.go)"},
		{name: "edit", body: formatToolInvocation("edit_file", `{"path":"internal/app.go"}`), want: "⬢ Edit (internal/app.go)"},
		{name: "read todo", body: formatToolInvocation("read_todo", `{}`), want: "⬢ Read TODO"},
		{name: "update todo", body: formatToolInvocation("update_todo", `{"content":"# TODO"}`), want: "⬢ Update TODO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rendered := renderEntry(entry{
				role:  "tool",
				title: "call " + tt.name,
				body:  tt.body,
			}, 100)
			if plain := stripANSI(rendered); !strings.Contains(plain, tt.want) {
				t.Fatalf("rendered tool call missing %q:\n%s", tt.want, rendered)
			}
		})
	}
}

func TestToolInvocationMalformedKnownArgsFallsBackSafely(t *testing.T) {
	got := formatToolInvocation("read_file", "{not-json")
	if !strings.Contains(got, "read_file") || !strings.Contains(got, "{not-json") {
		t.Fatalf("malformed known tool args fallback = %q, want tool name and raw args", got)
	}
}

func TestToolResultRenderingUsesTreePrefix(t *testing.T) {
	rendered := renderEntry(entry{
		role:  "tool",
		title: "result bash",
		body:  "first line\nsecond line",
	}, 100)
	plain := stripANSI(rendered)

	for _, want := range []string{"└─ first line", "   second line"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("rendered tool result missing %q:\n%s", want, rendered)
		}
	}
}

func TestToolResultRenderingShowsEmptyOutputFallback(t *testing.T) {
	rendered := renderEntry(entry{
		role:  "tool",
		title: "result bash",
	}, 100)
	if plain := stripANSI(rendered); !strings.Contains(plain, "└─ (no output)") {
		t.Fatalf("rendered empty tool result missing fallback:\n%s", rendered)
	}
}

func TestToolResultRenderingCollapsesLongOutput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = []entry{{
		role:  "tool",
		title: "result bash",
		body:  "line 1\nline 2\nline 3\nline 4\nline 5",
	}}

	plain := stripANSI(m.View())
	for _, want := range []string{"└─ line 1", "   line 2", "   line 3", "+2 lines (ctrl+o to expand)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("collapsed tool result missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "line 4") || strings.Contains(plain, "line 5") {
		t.Fatalf("collapsed tool result should hide lines after third:\n%s", plain)
	}
}

func TestCtrlOTogglesLongToolOutputExpansion(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = []entry{{
		role:  "tool",
		title: "result bash",
		body:  "line 1\nline 2\nline 3\nline 4\nline 5",
	}}

	m, _ = update(t, m, keyCtrlO())
	expanded := stripANSI(m.View())
	if strings.Contains(expanded, "ctrl+o to expand") {
		t.Fatalf("expanded output still shows collapse hint:\n%s", expanded)
	}
	for _, want := range []string{"line 4", "line 5"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded tool result missing %q:\n%s", want, expanded)
		}
	}

	m, _ = update(t, m, keyCtrlO())
	collapsed := stripANSI(m.View())
	if !strings.Contains(collapsed, "+2 lines (ctrl+o to expand)") || strings.Contains(collapsed, "line 5") {
		t.Fatalf("second ctrl+o should collapse output again:\n%s", collapsed)
	}
}

func TestShellCommandRenderingCollapsesLongOutput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = []entry{{
		role:  "command",
		title: "Shell: !printf lines",
		body:  "line 1\nline 2\nline 3\nline 4\nline 5",
	}}

	plain := stripANSI(m.View())
	for _, want := range []string{"Shell: !printf lines", "line 1", "line 2", "line 3", "+2 lines (ctrl+o to expand)"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("collapsed shell command output missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "line 4") || strings.Contains(plain, "line 5") {
		t.Fatalf("collapsed shell command output should hide lines after third:\n%s", plain)
	}
}

func TestCtrlOTogglesShellCommandOutputExpansion(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = []entry{{
		role:  "command",
		title: "Shell: !printf lines",
		body:  "line 1\nline 2\nline 3\nline 4\nline 5",
	}}

	m, _ = update(t, m, keyCtrlO())
	expanded := stripANSI(m.View())
	if strings.Contains(expanded, "ctrl+o to expand") {
		t.Fatalf("expanded shell output still shows collapse hint:\n%s", expanded)
	}
	for _, want := range []string{"line 4", "line 5"} {
		if !strings.Contains(expanded, want) {
			t.Fatalf("expanded shell output missing %q:\n%s", want, expanded)
		}
	}
}

func TestShellCommandFailureRenderingUsesErrorState(t *testing.T) {
	failed := renderCommandEntry(entry{
		role:  "command",
		title: "Shell: !false",
		body:  "exit status 1",
		err:   true,
	}, 80, true)
	succeeded := renderCommandEntry(entry{
		role:  "command",
		title: "Shell: !false",
		body:  "exit status 1",
	}, 80, true)

	if failed == succeeded {
		t.Fatalf("failed shell command rendering matches success rendering:\n%s", failed)
	}
	if !strings.Contains(stripANSI(failed), "exit status 1") {
		t.Fatalf("failed shell command rendering lost output:\n%s", failed)
	}
}

func TestShellCommandRenderingPreservesWhitespace(t *testing.T) {
	rendered := stripANSI(renderCommandEntry(entry{
		role:  "command",
		title: "Shell: !printf yaml",
		body:  "  key: value  \n\n",
	}, 80, true))

	if !strings.Contains(rendered, "    key: value  ") {
		t.Fatalf("rendered shell output did not preserve leading/trailing spaces:\n%q", rendered)
	}
	if !strings.Contains(rendered, "\n  \n") {
		t.Fatalf("rendered shell output did not preserve blank output line:\n%q", rendered)
	}
}

func TestFormatShellCommandResultTruncatesLargeOutput(t *testing.T) {
	raw := strings.Repeat("x", maxShellCommandOutputBytes*2)
	formatted := formatShellCommandResult(tools.BashCommandResult{Output: raw})

	if len(formatted) >= len(raw) {
		t.Fatalf("formatted output length = %d, want shorter than raw length %d", len(formatted), len(raw))
	}
	if !strings.Contains(formatted, "output truncated") {
		t.Fatalf("formatted output missing truncation marker: %q", formatted[len(formatted)-80:])
	}
}

func TestFormatShellCommandResultPreservesWhitespace(t *testing.T) {
	for _, output := range []string{
		"  key: value\n\n",
		" \n\t",
	} {
		formatted := formatShellCommandResult(tools.BashCommandResult{Output: output})
		if formatted != output {
			t.Fatalf("formatted output = %q, want original output %q", formatted, output)
		}
	}
}

func TestAssistantMessagesRenderMarkdown(t *testing.T) {
	rendered := renderAssistantEntry(entry{
		role:  "assistant",
		title: "foxharness",
		body:  "Today is **Sunday, May 17, 2026**.\n\n- current day\n- terminal markdown",
		time:  time.Date(2026, 5, 17, 15, 38, 44, 0, time.Local),
	}, 100)
	plainRendered := stripANSI(rendered)

	for _, forbidden := range []string{"**Sunday", "**."} {
		if strings.Contains(plainRendered, forbidden) {
			t.Fatalf("rendered assistant markdown contains raw markdown %q:\n%s", forbidden, rendered)
		}
	}
	for _, want := range []string{"Sunday, May 17, 2026", "current day", "terminal markdown"} {
		if !strings.Contains(plainRendered, want) {
			t.Fatalf("rendered assistant markdown missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(plainRendered, "15:38:44") {
		t.Fatalf("rendered assistant markdown contains timestamp:\n%s", rendered)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("rendered assistant markdown missing terminal styling escape codes:\n%s", rendered)
	}
}

func TestUserEntryRendersOnlyHighlightedBody(t *testing.T) {
	rendered := renderUserEntry(entry{
		role: "user",
		body: "inspect go.mod",
		time: time.Date(2026, 5, 17, 15, 38, 44, 0, time.Local),
	}, 80)
	plainRendered := stripANSI(rendered)

	for _, forbidden := range []string{"You", "15:38:44"} {
		if strings.Contains(plainRendered, forbidden) {
			t.Fatalf("rendered user entry contains redundant fragment %q:\n%s", forbidden, rendered)
		}
	}
	if !strings.Contains(plainRendered, "inspect go.mod") {
		t.Fatalf("rendered user entry missing body:\n%s", rendered)
	}
	if !strings.HasPrefix(plainRendered, "▌ ") {
		t.Fatalf("rendered user entry missing v5 left bar:\n%s", rendered)
	}
	if got := lipgloss.Width(strings.Split(plainRendered, "\n")[0]); got != 80 {
		t.Fatalf("rendered user entry width = %d, want 80:\n%s", got, rendered)
	}
}

func TestUserEntryRenderingPreservesPastedLineStructure(t *testing.T) {
	rendered := stripANSI(renderUserEntry(entry{
		role: "user",
		body: "• Ran go test ./internal/tui -count=1\n  └ ok github.com/Zts0hg/foxharness/internal/tui 1.835s\n\n• Explored",
	}, 90))

	for _, want := range []string{
		"• Ran go test ./internal/tui -count=1",
		"  └ ok github.com/Zts0hg/foxharness/internal/tui 1.835s",
		"• Explored",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered user entry missing preserved line %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "1.835s • Explored") {
		t.Fatalf("rendered user entry collapsed pasted lines:\n%s", rendered)
	}
}

func TestModelAcceptsSpaces(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("hello"))
	m, _ = update(t, m, keySpace())
	m, _ = update(t, m, keyRunes("world"))

	if got := string(m.input); got != "hello world" {
		t.Fatalf("input = %q, want hello world", got)
	}
}

func TestModelShiftEnterInsertsNewlineAndEnterSubmits(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("first line"))
	m, cmd := update(t, m, keyShiftEnter())
	if cmd != nil {
		t.Fatalf("shift+enter returned command")
	}
	m, _ = update(t, m, keyRunes("second line"))
	if got := string(m.input); got != "first line\nsecond line" {
		t.Fatalf("input = %q, want multiline text", got)
	}
	plainView := stripANSI(m.View())
	if !strings.Contains(plainView, "first line") || !strings.Contains(plainView, "second line") {
		t.Fatalf("view missing multiline input:\n%s", m.View())
	}

	m, cmd = update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("enter did not submit multiline prompt")
	}
	m, _ = update(t, m, cmd())
	if len(runner.runs) != 1 || runner.runs[0] != "first line\nsecond line" {
		t.Fatalf("runs = %#v, want multiline prompt", runner.runs)
	}
}

func TestModelRestoresVisibleHistoryAndInputHistory(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "remember TUI_RESUME_001"}),
		historyRecord(1, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "ok"}),
		historyRecord(2, "run-2", schema.Message{Role: schema.RoleUser, Content: "what did I ask you to remember?"}),
		historyRecord(3, "run-2", schema.Message{Role: schema.RoleAssistant, Content: "TUI_RESUME_001"}),
	}

	m := NewModel(context.Background(), runner, Config{})
	if m.status != "Resumed session: sess-1" {
		t.Fatalf("status = %q, want resumed status", m.status)
	}
	if entriesContain(m.entries, "system", "Interactive session started") {
		t.Fatalf("resumed model should not render fresh-session notice: %#v", m.entries)
	}
	for _, want := range []struct {
		role string
		body string
	}{
		{role: "user", body: "remember TUI_RESUME_001"},
		{role: "assistant", body: "ok"},
		{role: "user", body: "what did I ask you to remember?"},
		{role: "assistant", body: "TUI_RESUME_001"},
	} {
		if !entriesContain(m.entries, want.role, want.body) {
			t.Fatalf("restored entries missing %s %q: %#v", want.role, want.body, m.entries)
		}
	}

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "what did I ask you to remember?" {
		t.Fatalf("first restored history recall = %q, want latest user prompt", got)
	}
	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "remember TUI_RESUME_001" {
		t.Fatalf("second restored history recall = %q, want previous user prompt", got)
	}
}

func TestModelRestoresDisplayContentForPromptCommands(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		{
			Seq:            0,
			RunID:          "run-1",
			DisplayContent: "/review pr-9",
			Message:        schema.Message{Role: schema.RoleUser, Content: "Review: pr-9"},
		},
		historyRecord(1, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "done"}),
	}

	m := NewModel(context.Background(), runner, Config{})
	if !entriesContain(m.entries, "user", "/review pr-9") {
		t.Fatalf("restored entries missing display content: %#v", m.entries)
	}
	if entriesContain(m.entries, "user", "Review: pr-9") {
		t.Fatalf("restored entries rendered model content: %#v", m.entries)
	}

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "/review pr-9" {
		t.Fatalf("restored input history = %q, want original command", got)
	}
}

func TestModelRestoresToolHistoryWhenAvailable(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "what day is it?"}),
		historyRecord(1, "run-1", schema.Message{
			Role: schema.RoleAssistant,
			ToolCalls: []schema.ToolCall{{
				ID:        "call-1",
				Name:      "bash",
				Arguments: json.RawMessage(`{"command":"date"}`),
			}},
		}),
		historyRecord(2, "run-1", schema.Message{Role: schema.RoleUser, ToolCallID: "call-1", Content: "2026年 5月17日 星期日"}),
		historyRecord(3, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "Today is Sunday."}),
	}

	m := NewModel(context.Background(), runner, Config{})
	if !entriesContain(m.entries, "tool", "Bash (date)") {
		t.Fatalf("restored entries missing tool call: %#v", m.entries)
	}
	if !entriesContain(m.entries, "tool", "2026年 5月17日") {
		t.Fatalf("restored entries missing tool result: %#v", m.entries)
	}
	if !entriesContain(m.entries, "assistant", "Today is Sunday.") {
		t.Fatalf("restored entries missing assistant response: %#v", m.entries)
	}
}

func TestModelSkipsCompactionSummaryDuringRestore(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "first real prompt"}),
		historyRecord(1, "run-1", schema.Message{Role: schema.RoleUser, Content: "## Compacted Context Summary\n\nold facts"}),
		historyRecord(2, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "done"}),
	}

	m := NewModel(context.Background(), runner, Config{})
	if entriesContain(m.entries, "user", "Compacted Context Summary") {
		t.Fatalf("compaction summary should not render in restored transcript: %#v", m.entries)
	}
	if len(m.inputHistory) != 1 || m.inputHistory[0] != "first real prompt" {
		t.Fatalf("inputHistory = %#v, want only real user prompt", m.inputHistory)
	}
}

func TestModelShowsHistoryLoadError(t *testing.T) {
	runner := newFakeRunner()
	runner.historyErr = errors.New("history unavailable")

	m := NewModel(context.Background(), runner, Config{})
	if m.status != "History load failed" {
		t.Fatalf("status = %q, want history load failure", m.status)
	}
	if !entriesContain(m.entries, "error", "history unavailable") {
		t.Fatalf("entries missing history load error: %#v", m.entries)
	}
}

func TestNewSessionClearsTranscriptAndRefreshesProjectHistory(t *testing.T) {
	runner := &projectHistoryRunner{fakeRunner: newFakeRunner(), projectHistory: []string{"previous session prompt"}}
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "old prompt"}),
		historyRecord(1, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "old answer"}),
	}
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/new"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("/new command is nil")
	}
	m, _ = update(t, m, cmd())

	if entriesContain(m.entries, "user", "old prompt") || entriesContain(m.entries, "assistant", "old answer") {
		t.Fatalf("/new should clear restored transcript entries: %#v", m.entries)
	}
	if !entriesContain(m.entries, "command", "ID       sess-new") {
		t.Fatalf("/new did not render new session details: %#v", m.entries)
	}
	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "previous session prompt" {
		t.Fatalf("Up after /new restored %q, want previous session prompt", got)
	}
}

func TestModelSlashCommands(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/help"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/help returned unexpected command")
	}
	if !entriesContain(m.entries, "command", "/status") ||
		!entriesContain(m.entries, "command", "show session status overview") ||
		!entriesContain(m.entries, "command", "/session") {
		t.Fatalf("/help did not render commands: %#v", m.entries)
	}

	m, _ = update(t, m, keyRunes("/session"))
	m, _ = update(t, m, keyEnter())
	if !entriesContain(m.entries, "command", "Session") ||
		!entriesContain(m.entries, "command", "Runtime") ||
		!entriesContain(m.entries, "command", "Capabilities") {
		t.Fatalf("/session did not render status overview: %#v", m.entries)
	}

	m, _ = update(t, m, keyRunes("/clear"))
	m, cmd = update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("/clear command is nil, want new-session command")
	}
	m, _ = update(t, m, cmd())
	if m.sessionID != "sess-new" {
		t.Fatalf("/clear sessionID = %q, want sess-new", m.sessionID)
	}
	if !entriesContain(m.entries, "command", "ID       sess-new") {
		t.Fatalf("/clear did not render new session details: %#v", m.entries)
	}

	m, _ = update(t, m, keyRunes("/new"))
	m, cmd = update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("/new command is nil")
	}
	m, _ = update(t, m, cmd())
	if m.sessionID != "sess-new" {
		t.Fatalf("sessionID = %q, want sess-new", m.sessionID)
	}
	if !entriesContain(m.entries, "command", "ID       sess-new") {
		t.Fatalf("/new did not render switch message: %#v", m.entries)
	}
}

func TestModelSlashCommandEffortOpensSelector(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{ProviderProtocol: "openai"})

	next, _ := m.handleSlashCommand("/effort")
	m = next.(Model)

	if m.effortForm == nil {
		t.Fatal("effort form = nil, want selector")
	}
	if len(m.effortForm.options) != 7 || m.effortForm.options[1] != "none" || m.effortForm.options[2] != "minimal" {
		t.Fatalf("options = %v, want openai effort options", m.effortForm.options)
	}
}

func TestModelSlashCommandEffortSelectsSessionOverride(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{ProviderProtocol: "openai", EffortOverride: "minimal"})

	next, _ := m.handleSlashCommand("/effort")
	m = next.(Model)

	if m.effortValue != "minimal" {
		t.Fatalf("effortValue = %q, want minimal", m.effortValue)
	}
	if m.effortForm == nil {
		t.Fatal("effort form = nil, want selector")
	}
	if got := m.effortForm.options[m.effortForm.cursor]; got != "minimal" {
		t.Fatalf("selected effort = %q, want minimal", got)
	}
}

func TestModelSlashCommandEffortWithArgumentDoesNotSet(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{ProviderProtocol: "openai"})

	next, _ := m.handleSlashCommand("/effort high")
	m = next.(Model)

	if m.effortForm != nil {
		t.Fatal("effort form opened for /effort high, want selector-only error")
	}
	if m.effortValue != "auto" {
		t.Fatalf("effortValue = %q, want auto", m.effortValue)
	}
	if !entriesContain(m.entries, "error", "Usage: /effort") {
		t.Fatalf("entries = %#v, want usage error", m.entries)
	}
}

func TestStatusCommandRendersGroupedOverview(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{
		ProviderID:        "openai",
		ProviderProfileID: "primary",
		ProviderProtocol:  "openai",
	})
	m.queuedPrompts = testQueuedPrompts("queued follow-up")
	m.sidebarVisible = false

	m, _ = update(t, m, keyRunes("/status"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/status returned command, want nil")
	}

	plain := stripANSI(renderEntry(m.entries[len(m.entries)-1], 100))
	for _, want := range []string{
		"Status",
		"Session",
		"ID",
		"sess-1",
		"Dir",
		"/tmp/sess-1",
		"Workdir",
		"/tmp/work",
		"Git",
		"Model",
		"Provider",
		"openai",
		"Profile",
		"primary",
		"Plan Mode",
		"Runtime",
		"Queued Prompts",
		"1",
		"Context",
		"7%",
		"UI",
		"Theme",
		"codex",
		"Statusline",
		"model, project, git-branch, context-used",
		"Sidebar",
		"hidden",
		"Capabilities",
		"Rewind",
		"Ask User",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("/status missing %q:\n%s", want, plain)
		}
	}
}

func TestStatusCommandReportsInlineProviderWithoutProfile(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{
		ProviderID:       "typo",
		ProviderProtocol: "openai",
	})

	m, _ = update(t, m, keyRunes("/status"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/status returned command, want nil")
	}

	lines := strings.Split(stripANSI(renderEntry(m.entries[len(m.entries)-1], 100)), "\n")
	if !lineContainsAll(lines, "Profile", "inline") {
		t.Fatalf("/status did not report inline profile:\n%s", strings.Join(lines, "\n"))
	}
	if lineContainsAll(lines, "Profile", "typo") {
		t.Fatalf("/status reported provider id as profile:\n%s", strings.Join(lines, "\n"))
	}
}

func TestSessionCommandAliasesStatusOverview(t *testing.T) {
	runner := newFakeRunner()
	statusModel := NewModel(context.Background(), runner, Config{})
	statusModel, _ = update(t, statusModel, keyRunes("/status"))
	statusModel, _ = update(t, statusModel, keyEnter())

	runner = newFakeRunner()
	sessionModel := NewModel(context.Background(), runner, Config{})
	sessionModel, _ = update(t, sessionModel, keyRunes("/session"))
	sessionModel, _ = update(t, sessionModel, keyEnter())

	if len(statusModel.entries) == 0 || len(sessionModel.entries) == 0 {
		t.Fatalf("missing status/session entries: %#v %#v", statusModel.entries, sessionModel.entries)
	}
	statusEntry := statusModel.entries[len(statusModel.entries)-1]
	sessionEntry := sessionModel.entries[len(sessionModel.entries)-1]
	if statusEntry.title != sessionEntry.title || statusEntry.body != sessionEntry.body {
		t.Fatalf("/session not alias of /status:\nstatus=%#v\nsession=%#v", statusEntry, sessionEntry)
	}
}

func TestDeferredSlashCommandsRemainUnsupported(t *testing.T) {
	for _, command := range []string{"/review", "/vim", "/keymap"} {
		t.Run(command, func(t *testing.T) {
			runner := newFakeRunner()
			m := NewModel(context.Background(), runner, Config{})

			m, _ = update(t, m, keyRunes(command))
			m, cmd := update(t, m, keyEnter())
			if cmd != nil {
				t.Fatalf("%s returned command, want nil", command)
			}
			if !entriesContain(m.entries, "error", "Unknown command: "+command) {
				t.Fatalf("%s did not render unsupported command error: %#v", command, m.entries)
			}
		})
	}
}

func TestStatuslineDefaultsRenderConfiguredItems(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	plain := stripANSI(m.renderStatusBar(120))
	for _, want := range []string{
		"fake-model",
		"work",
		"git",
		"Context 7%",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("default statusline missing %q:\n%s", want, plain)
		}
	}
	for _, forbidden := range []string{"sid sess-1", "Run State", "plan mode"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("default statusline contains non-default item %q:\n%s", forbidden, plain)
		}
	}

	m.statuslineItems = []string{"plan-mode"}
	plain = stripANSI(m.renderStatusBar(120))
	if !strings.Contains(plain, "plan mode off") {
		t.Fatalf("configured plan-mode statusline missing explicit plan state:\n%s", plain)
	}
}

func TestStatuslineCommandListsAvailableItemsAndDefaults(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/statusline"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/statusline returned command, want nil")
	}

	plain := stripANSI(renderEntry(m.entries[len(m.entries)-1], 120))
	for _, want := range []string{
		"Statusline",
		"Current",
		"model, project, git-branch, context-used",
		"Default",
		"Available",
		"plan-mode",
		"run-state",
		"Usage: /statusline set <items>",
	} {
		if !strings.Contains(plain, want) {
			t.Fatalf("/statusline help missing %q:\n%s", want, plain)
		}
	}
}

func TestStatuslineSetPersistsOrderedItems(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	m, _ = update(t, m, keyRunes("/statusline set theme, queued session-id"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/statusline set returned command, want nil")
	}

	wantItems := []string{"theme", "queued", "session-id"}
	if !reflect.DeepEqual(m.statuslineItems, wantItems) {
		t.Fatalf("statuslineItems = %#v, want %#v", m.statuslineItems, wantItems)
	}
	plain := stripANSI(m.renderStatusBar(120))
	for _, want := range []string{"theme codex", "queued 0", "sid sess-1"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("configured statusline missing %q:\n%s", want, plain)
		}
	}
	tui := readTUISettingsMap(t, home)
	if !reflect.DeepEqual(tui["statusline"], []any{"theme", "queued", "session-id"}) {
		t.Fatalf("persisted statusline = %#v, want ordered items", tui["statusline"])
	}
}

func TestStatuslineDefaultRestoresAndPersistsDefaults(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})
	m.statuslineItems = []string{"theme", "queued"}

	m, _ = update(t, m, keyRunes("/statusline default"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/statusline default returned command, want nil")
	}

	if !reflect.DeepEqual(m.statuslineItems, defaultStatuslineItems) {
		t.Fatalf("statuslineItems = %#v, want defaults %#v", m.statuslineItems, defaultStatuslineItems)
	}
	tui := readTUISettingsMap(t, home)
	if !reflect.DeepEqual(tui["statusline"], []any{"model", "project", "git-branch", "context-used"}) {
		t.Fatalf("persisted defaults = %#v", tui["statusline"])
	}
}

func TestStatuslineRejectsShellHookLikeInputWithoutWriting(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})
	before := append([]string(nil), m.statuslineItems...)

	m, _ = update(t, m, keyRunes("/statusline set shell:/tmp/statusline.sh"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/statusline invalid returned command, want nil")
	}

	if !reflect.DeepEqual(m.statuslineItems, before) {
		t.Fatalf("statuslineItems changed on invalid input: %#v want %#v", m.statuslineItems, before)
	}
	if !entriesContain(m.entries, "error", "Unknown statusline item") {
		t.Fatalf("missing invalid statusline error: %#v", m.entries)
	}
	if _, err := os.Stat(settingsJSONPath(home)); !os.IsNotExist(err) {
		t.Fatalf("settings file exists after invalid statusline input: %v", err)
	}
}

func TestThemeCommandAppliesBuiltInThemeAndPersists(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	m, _ = update(t, m, keyRunes("/theme mono"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/theme returned command, want nil")
	}

	if m.themeName != "mono" {
		t.Fatalf("themeName = %q, want mono", m.themeName)
	}
	if !entriesContain(m.entries, "command", "Theme set to mono") {
		t.Fatalf("missing theme command entry: %#v", m.entries)
	}
	tui := readTUISettingsMap(t, home)
	if tui["theme"] != "mono" {
		t.Fatalf("persisted theme = %#v, want mono", tui["theme"])
	}
}

func TestThemeCommandRejectsInvalidThemeWithoutPersisting(t *testing.T) {
	home := t.TempDir()
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	m, _ = update(t, m, keyRunes("/theme solarized"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/theme invalid returned command, want nil")
	}

	if m.themeName != defaultThemeName {
		t.Fatalf("themeName = %q, want default %q", m.themeName, defaultThemeName)
	}
	if !entriesContain(m.entries, "error", "Unknown theme") {
		t.Fatalf("missing invalid theme error: %#v", m.entries)
	}
	if _, err := os.Stat(settingsJSONPath(home)); !os.IsNotExist(err) {
		t.Fatalf("settings file exists after invalid theme: %v", err)
	}
}

func TestNewModelRestoresThemeAndStatuslineFromSettings(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, home, ".foxharness/settings.json", `{
	  "tui": {
	    "theme": "mono",
	    "statusline": ["theme", "queued"]
	  }
	}`)
	runner := newFakeRunner()

	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	if m.themeName != "mono" {
		t.Fatalf("themeName = %q, want mono", m.themeName)
	}
	wantItems := []string{"theme", "queued"}
	if !reflect.DeepEqual(m.statuslineItems, wantItems) {
		t.Fatalf("statuslineItems = %#v, want %#v", m.statuslineItems, wantItems)
	}
	plain := stripANSI(m.renderStatusBar(120))
	for _, want := range []string{"theme mono", "queued 0"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("restored statusline missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "fake-model") {
		t.Fatalf("restored statusline rendered non-configured model item:\n%s", plain)
	}
}

func TestNewModelMigratesOldDefaultStatuslineWithoutPlanMode(t *testing.T) {
	home := t.TempDir()
	writeTestFile(t, home, ".foxharness/settings.json", `{
	  "tui": {
	    "statusline": ["model", "project", "git-branch", "context-used", "plan-mode"]
	  }
	}`)
	runner := newFakeRunner()

	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	if !reflect.DeepEqual(m.statuslineItems, defaultStatuslineItems) {
		t.Fatalf("statuslineItems = %#v, want migrated defaults %#v", m.statuslineItems, defaultStatuslineItems)
	}
	if strings.Contains(stripANSI(m.renderStatusBar(120)), "plan mode") {
		t.Fatalf("old default statusline should not keep plan-mode by default:\n%s", stripANSI(m.renderStatusBar(120)))
	}
}

func TestThemeCommandReportsPersistenceError(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".foxharness"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	m, _ = update(t, m, keyRunes("/theme mono"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/theme returned command, want nil")
	}

	if m.themeName != defaultThemeName {
		t.Fatalf("themeName = %q, want unchanged default %q", m.themeName, defaultThemeName)
	}
	if !entriesContain(m.entries, "error", "save settings") {
		t.Fatalf("missing settings persistence error: %#v", m.entries)
	}
}

func TestThemeCommandReportsMissingHomeDir(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/theme mono"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/theme returned command, want nil")
	}

	if m.themeName != defaultThemeName {
		t.Fatalf("themeName = %q, want unchanged default %q", m.themeName, defaultThemeName)
	}
	if !entriesContain(m.entries, "error", "home directory unavailable") {
		t.Fatalf("missing home-dir persistence error: %#v", m.entries)
	}
}

func TestStatuslineSetReportsMissingHomeDir(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	before := append([]string(nil), m.statuslineItems...)

	m, _ = update(t, m, keyRunes("/statusline set theme,queued"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/statusline returned command, want nil")
	}

	if !reflect.DeepEqual(m.statuslineItems, before) {
		t.Fatalf("statuslineItems = %#v, want unchanged %#v", m.statuslineItems, before)
	}
	if !entriesContain(m.entries, "error", "home directory unavailable") {
		t.Fatalf("missing home-dir persistence error: %#v", m.entries)
	}
}

func TestApplyThemeUpdatesOverlayStyles(t *testing.T) {
	t.Cleanup(func() { applyTheme(defaultThemeName) })

	applyTheme("mono")
	if got := askFocusedStyle.GetForeground(); got != cAccentHi {
		t.Fatalf("ask focused foreground = %q, want current accent highlight %q", got, cAccentHi)
	}

	view := selector.New([]checkpoint.SelectableMessage{{
		Seq:     7,
		Content: "restore this",
	}}, &tuiCheckpointer{}).View()
	wantTitle := lipgloss.NewStyle().Bold(true).Foreground(cAccentHi).Render("Rewind")
	if !strings.Contains(view, wantTitle) {
		t.Fatalf("selector title did not use current theme highlight %q:\n%s", cAccentHi, view)
	}
}

func TestModelSlashCommandCompact(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/compact"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("/compact should return a command")
	}
	if !m.running {
		t.Fatalf("model should be in running state during compact")
	}

	m, _ = update(t, m, cmd())
	if m.running {
		t.Fatalf("model should not be running after compact finishes")
	}
	if !entriesContain(m.entries, "command", "Summarized 15 messages") {
		t.Fatalf("/compact did not render result: %#v", m.entries)
	}
	if m.status != "Context compacted" {
		t.Fatalf("status = %q, want 'Context compacted'", m.status)
	}
}

func TestModelSlashCommandCompactWithInstructions(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/compact focus on migration"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("/compact should return a command")
	}

	_ = cmd()
	if runner.compactInstr != "focus on migration" {
		t.Fatalf("compactInstr = %q, want 'focus on migration'", runner.compactInstr)
	}
}

func TestModelSlashCommandCompactWhileRunning(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true

	m, _ = update(t, m, keyRunes("/compact"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/compact while running should not dispatch a command")
	}
	if m.status != "Cannot compact while a run is active" {
		t.Fatalf("status = %q, want busy message", m.status)
	}
}

func TestModelSlashCommandCompactError(t *testing.T) {
	runner := newFakeRunner()
	runner.compactErr = errors.New("boom")
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/compact"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("/compact should return a command")
	}

	m, _ = update(t, m, cmd())
	if !entriesContain(m.entries, "error", "boom") {
		t.Fatalf("/compact error not rendered: %#v", m.entries)
	}
	if m.status != "Compact failed" {
		t.Fatalf("status = %q, want 'Compact failed'", m.status)
	}
}

func TestSlashCommandsOpenRewindSelector(t *testing.T) {
	cp := &tuiCheckpointer{stats: &checkpoint.DiffStats{FilesChanged: 1}}
	runner := newFakeRunner()
	runner.checkpointer = cp
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "restore this"}),
		historyRecord(1, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "done"}),
	}
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/rewind"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/rewind returned unexpected command")
	}
	if m.rewindSelector == nil {
		t.Fatalf("/rewind did not open selector")
	}
	plain := stripANSI(m.View())
	if !strings.Contains(plain, "restore this") {
		t.Fatalf("selector view missing user message:\n%s", plain)
	}

	m, _ = update(t, m, selector.ResultMsg{Action: selector.ActionRestoreBoth, MessageID: "0"})
	if cp.rewind != "0" {
		t.Fatalf("Rewind called with %q, want 0", cp.rewind)
	}
	if runner.truncatedSeq != 0 {
		t.Fatalf("truncated seq = %d, want 0", runner.truncatedSeq)
	}
	if runner.restoreStateSeq != 0 {
		t.Fatalf("restoreStateSeq = %d, want 0", runner.restoreStateSeq)
	}
	if got := string(m.input); got != "restore this" {
		t.Fatalf("input after rewind = %q, want restore this", got)
	}

	m.running = true
	m.input = nil
	m, _ = update(t, m, keyRunes("/checkpoint"))
	m, _ = update(t, m, keyEnter())
	if !strings.Contains(m.status, "unavailable") {
		t.Fatalf("status = %q, want rewind unavailable while running", m.status)
	}
}

func TestDoubleEscClearsInputAndRecordsHistory(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.Local)
	m.now = func() time.Time { return now }
	m.input = []rune("draft")

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if got := string(m.input); got != "draft" {
		t.Fatalf("first esc input = %q, want draft", got)
	}
	if !strings.Contains(m.status, "Esc again to clear") {
		t.Fatalf("first esc status = %q, want clear prompt", m.status)
	}
	if m.rewindSelector != nil {
		t.Fatalf("first esc opened rewind selector")
	}

	now = now.Add(time.Second)
	m, cmd := update(t, m, keyEsc())
	m, cmd = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if cmd != nil {
		t.Fatalf("second esc returned unexpected command")
	}
	if got := string(m.input); got != "" {
		t.Fatalf("second esc input = %q, want cleared", got)
	}
	if len(m.inputHistory) != 1 || m.inputHistory[0] != "draft" {
		t.Fatalf("inputHistory = %#v, want draft", m.inputHistory)
	}

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "draft" {
		t.Fatalf("up after clear restored %q, want draft", got)
	}
}

func TestDoubleEscOpensRewindSelectorWhenInputEmpty(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "restore this"}),
	}
	m := NewModel(context.Background(), runner, Config{})
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.Local)
	m.now = func() time.Time { return now }

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if m.rewindSelector != nil {
		t.Fatalf("first esc opened rewind selector")
	}
	if !strings.Contains(m.status, "Press Esc again") {
		t.Fatalf("first esc status = %q, want second-esc rewind prompt", m.status)
	}

	now = now.Add(time.Second)
	m, cmd := update(t, m, keyEsc())
	m, cmd = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if cmd != nil {
		t.Fatalf("second esc returned unexpected command")
	}
	if m.rewindSelector == nil {
		t.Fatalf("second esc did not open rewind selector")
	}
	if !strings.Contains(stripANSI(m.View()), "restore this") {
		t.Fatalf("selector view missing rewind target:\n%s", m.View())
	}
}

func TestDoubleEscClearExpiresAfterConfirmWindow(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.Local)
	m.now = func() time.Time { return now }
	m.input = []rune("draft")

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	now = now.Add(quitConfirmWindow + time.Millisecond)
	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})

	if got := string(m.input); got != "draft" {
		t.Fatalf("expired second esc input = %q, want draft", got)
	}
	if len(m.inputHistory) != 0 {
		t.Fatalf("inputHistory = %#v, want empty", m.inputHistory)
	}
	if !strings.Contains(m.status, "Esc again to clear") {
		t.Fatalf("status = %q, want clear prompt", m.status)
	}
}

func TestDoubleEscClearThenRewindRequiresTwoMoreEscapes(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "restore this"}),
	}
	m := NewModel(context.Background(), runner, Config{})
	now := time.Date(2026, 5, 22, 12, 0, 0, 0, time.Local)
	m.now = func() time.Time { return now }
	m.input = []rune("draft")

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	now = now.Add(time.Second)
	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if m.rewindSelector != nil {
		t.Fatalf("clear opened rewind selector")
	}

	now = now.Add(time.Second)
	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if m.rewindSelector != nil {
		t.Fatalf("first empty esc after clear opened rewind selector")
	}

	now = now.Add(time.Second)
	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if m.rewindSelector == nil {
		t.Fatalf("second empty esc after clear did not open rewind selector")
	}
}

func TestAutoRestoreOnCancel(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "cancel me"}),
		{Seq: 1, RunID: "run-1", Kind: "progress", Message: schema.Message{Role: schema.RoleAssistant}},
	}
	m := NewModel(context.Background(), runner, Config{})
	m.running = true
	cancelled := false
	m.cancelRun = func() { cancelled = true }

	m, _ = update(t, m, keyCtrlC())
	if !cancelled {
		t.Fatalf("ctrl+c did not call cancelRun")
	}
	if runner.truncatedSeq != 0 {
		t.Fatalf("truncated seq = %d, want 0", runner.truncatedSeq)
	}
	if got := string(m.input); got != "cancel me" {
		t.Fatalf("input after auto restore = %q, want cancel me", got)
	}
}

func TestNoAutoRestoreWithMeaningfulContent(t *testing.T) {
	runner := newFakeRunner()
	runner.history = []session.MessageRecord{
		historyRecord(0, "run-1", schema.Message{Role: schema.RoleUser, Content: "do work"}),
		historyRecord(1, "run-1", schema.Message{Role: schema.RoleAssistant, Content: "started"}),
	}
	m := NewModel(context.Background(), runner, Config{})
	m.running = true
	m.cancelRun = func() {}

	m, _ = update(t, m, keyCtrlC())
	if runner.truncatedSeq != -1 {
		t.Fatalf("truncated seq = %d, want no truncate", runner.truncatedSeq)
	}
	if got := string(m.input); got != "" {
		t.Fatalf("input after no auto restore = %q, want empty", got)
	}
}

func TestModelSlashSuggestionsAndTabCompletion(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m.inputHistory = []string{"previous task"}
	m, _ = update(t, m, keyRunes("/"))
	view := m.View()
	plainView := stripANSI(view)
	for _, want := range []string{"❯", "/status", "/session", "/clear", "/new", "/model", "/theme", "/statusline", "/cancel", "/sidebar", "/help", "/exit"} {
		if !strings.Contains(plainView, want) {
			t.Fatalf("view missing slash dropdown item %q:\n%s", want, view)
		}
	}
	if strings.Contains(plainView, "Tab complete  /") {
		t.Fatalf("slash dropdown should not render old inline hint:\n%s", view)
	}

	m, _ = update(t, m, keyDown())
	if got := string(m.input); got != "/" {
		t.Fatalf("down in slash menu changed input = %q, want /", got)
	}
	if command, ok := m.selectedSlashCommand(); !ok || command.Name != "/session" {
		t.Fatalf("selected slash command after down = %#v, %v; want /session", command, ok)
	}
	m, _ = update(t, m, keyUp())
	if command, ok := m.selectedSlashCommand(); !ok || command.Name != "/status" {
		t.Fatalf("selected slash command after up = %#v, %v; want /status", command, ok)
	}

	m, _ = update(t, m, keyTab())
	if got := string(m.input); got != "/status" {
		t.Fatalf("input after tab = %q, want /status", got)
	}
}

func TestModelSidebarCommandTogglesSidebar(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", "plan")
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	if !strings.Contains(stripANSI(m.View()), sidebarHintText) {
		t.Fatalf("visible sidebar should render hide hint:\n%s", m.View())
	}

	m, _ = update(t, m, keyRunes("/sidebar off"))
	m, _ = update(t, m, keyEnter())
	if m.sidebarVisible {
		t.Fatalf("/sidebar off left sidebarVisible=true")
	}
	plainView := stripANSI(m.View())
	if strings.Contains(plainView, "MEMORY") || strings.Contains(plainView, sidebarHintText) {
		t.Fatalf("/sidebar off should hide sidebar and hint:\n%s", plainView)
	}

	m, _ = update(t, m, keyRunes("/sidebar on"))
	m, _ = update(t, m, keyEnter())
	if !m.sidebarVisible {
		t.Fatalf("/sidebar on left sidebarVisible=false")
	}
	plainView = stripANSI(m.View())
	if !strings.Contains(plainView, "MEMORY") || !strings.Contains(plainView, sidebarHintText) {
		t.Fatalf("/sidebar on should show sidebar and hint:\n%s", plainView)
	}
}

func TestModelSlashDropdownEnterRunsSelectedCommand(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/"))
	m, _ = update(t, m, keyEnter())

	if string(m.input) != "" {
		t.Fatalf("input after selected slash command = %q, want empty", string(m.input))
	}
	if !entriesContain(m.entries, "command", "ID       sess-1") {
		t.Fatalf("enter did not execute selected /session command: %#v", m.entries)
	}
}

func TestModelMouseWheelScrollsTranscript(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if m.scrollOffset != 3 {
		t.Fatalf("scrollOffset after wheel up = %d, want 3", m.scrollOffset)
	}

	m, _ = update(t, m, tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.scrollOffset != 0 {
		t.Fatalf("scrollOffset after wheel down = %d, want 0", m.scrollOffset)
	}

	m, _ = update(t, m, tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.scrollOffset != 0 {
		t.Fatalf("scrollOffset after extra wheel down = %d, want clamped 0", m.scrollOffset)
	}
}

func TestTranscriptPointAtUsesVisibleScrollOffset(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = nil
	for i := 0; i < 12; i++ {
		m.appendCommandEntry(fmt.Sprintf("line %02d", i), fmt.Sprintf("body %02d", i))
	}
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m.scrollOffset = 2

	layout := m.transcriptLayout(m.chatWidth(), m.transcriptHeight())
	localY := 0
	for i := layout.visibleStart; i < layout.visibleEnd; i++ {
		if len(layout.plainLines[i]) >= 8 {
			localY = i - layout.visibleStart
			break
		}
	}
	point, ok := m.transcriptPointAt(viewPaddingLeft+3, viewPaddingTop+localY, false)
	if !ok {
		t.Fatalf("transcript point did not map")
	}
	if point.line != layout.visibleStart+localY || point.col != 3 {
		t.Fatalf("point = %+v, want line %d col 3", point, layout.visibleStart+localY)
	}
}

func TestTranscriptDragSelectionCopiesSingleLine(t *testing.T) {
	m, copied := selectableTranscriptModel(t, "alpha beta gamma")
	line, col := findTranscriptText(t, m, "alpha beta gamma")

	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + col, Y: viewPaddingTop + line, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + col + len("alpha beta"), Y: viewPaddingTop + line, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + col + len("alpha beta"), Y: viewPaddingTop + line, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	if *copied != "alpha beta" {
		t.Fatalf("copied = %q, want alpha beta", *copied)
	}
}

func TestTranscriptDragSelectionCopiesMultipleLinesAndReverse(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = nil
	m.appendCommandEntry("selection fixture", "alpha beta\ngamma delta")
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	copied := ""
	m.copySelection = func(text string) error {
		copied = text
		return nil
	}
	firstLine, firstCol := findTranscriptText(t, m, "alpha beta")
	secondLine, secondCol := findTranscriptText(t, m, "gamma delta")

	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + secondCol + len("gamma"), Y: viewPaddingTop + secondLine, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + firstCol + len("alpha "), Y: viewPaddingTop + firstLine, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + firstCol + len("alpha "), Y: viewPaddingTop + firstLine, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	want := "beta\n  gamma"
	if copied != want {
		t.Fatalf("copied = %q, want %q", copied, want)
	}
}

func TestTranscriptDragSelectionEmptyDoesNotCopy(t *testing.T) {
	m, copied := selectableTranscriptModel(t, "alpha beta")
	line, col := findTranscriptText(t, m, "alpha beta")

	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + col, Y: viewPaddingTop + line, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + col, Y: viewPaddingTop + line, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	if *copied != "" {
		t.Fatalf("copied = %q, want empty", *copied)
	}
	if m.selection.active {
		t.Fatalf("empty selection should be cleared")
	}
}

func TestTranscriptDragSelectionClampsOutsideBounds(t *testing.T) {
	m, copied := selectableTranscriptModel(t, "alpha beta")
	line, col := findTranscriptText(t, m, "alpha beta")

	m, _ = update(t, m, tea.MouseMsg{X: viewPaddingLeft + col + len("alpha "), Y: viewPaddingTop + line, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, _ = update(t, m, tea.MouseMsg{X: -10, Y: -10, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, _ = update(t, m, tea.MouseMsg{X: -10, Y: -10, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	if *copied != "alpha " {
		t.Fatalf("copied = %q, want %q", *copied, "alpha ")
	}
}

func TestInputDragSelectionCopiesInputText(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("alpha beta gamma")
	m.inputCursor = len(m.input)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	copied := ""
	m.copySelection = func(text string) error {
		copied = text
		return nil
	}

	y, x := inputTextOrigin(m)
	start := x + len("alpha ")
	end := start + len("beta")
	m, _ = update(t, m, tea.MouseMsg{X: start, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, _ = update(t, m, tea.MouseMsg{X: end, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, _ = update(t, m, tea.MouseMsg{X: end, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	if copied != "beta" {
		t.Fatalf("copied = %q, want beta", copied)
	}
	if m.status != "Selection copied" {
		t.Fatalf("status = %q, want Selection copied", m.status)
	}
}

func TestTranscriptSelectionHighlightRendersVisibleRange(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = nil
	for i := 0; i < 30; i++ {
		body := fmt.Sprintf("line %02d", i)
		if i == 29 {
			body = "alpha beta"
		}
		m.appendCommandEntry(fmt.Sprintf("entry %02d", i), body)
	}
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	line, col := findTranscriptText(t, m, "alpha beta")
	layout := m.transcriptLayout(m.chatWidth(), m.transcriptHeight())
	m.selection = selectionState{
		anchor: selectionPoint{line: layout.visibleStart + line, col: col + len("alpha ")},
		focus:  selectionPoint{line: layout.visibleStart + line, col: col + len("alpha beta")},
		active: true,
	}

	rendered := m.renderBody(m.chatWidth(), m.transcriptHeight())
	if !strings.Contains(rendered, selectionStyle.Render("beta")) {
		t.Fatalf("rendered body does not contain highlighted selection:\n%s", rendered)
	}

	m.scrollOffset = 8
	rendered = m.renderBody(m.chatWidth(), m.transcriptHeight())
	if strings.Contains(rendered, selectionStyle.Render("beta")) {
		t.Fatalf("scrolled body should not highlight non-visible selection:\n%s", rendered)
	}
}

func TestTranscriptSelectionHighlightOverridesMarkdownStyling(t *testing.T) {
	m, _ := selectableTranscriptModel(t, "## foxharness 项目定位分析")
	line, col := findTranscriptText(t, m, "foxharness")
	layout := m.transcriptLayout(m.chatWidth(), m.transcriptHeight())
	m.selection = selectionState{
		anchor: selectionPoint{line: layout.visibleStart + line, col: col},
		focus:  selectionPoint{line: layout.visibleStart + line, col: col + len("foxharness")},
		active: true,
	}

	rendered := m.renderBody(m.chatWidth(), m.transcriptHeight())
	if !strings.Contains(rendered, selectionStyle.Render("foxharness")) {
		t.Fatalf("rendered markdown heading should contain plain highlighted selected text:\n%s", rendered)
	}
}

func TestFragmentedSGRMousePayloadDoesNotEnterInput(t *testing.T) {
	runner := newFakeRunner()
	for _, payload := range []string{
		"[<64;57;23M",
		"[<65;72;19M",
		"[>64;55;16M",
		"[<80;57;23M",
	} {
		t.Run(payload, func(t *testing.T) {
			m := NewModel(context.Background(), runner, Config{})
			m.input = []rune("draft")
			m.scrollOffset = 1

			m, _ = update(t, m, keyRunes(payload))

			if got := string(m.input); got != "draft" {
				t.Fatalf("input after fragmented mouse payload = %q, want draft", got)
			}
		})
	}
}

func TestFragmentedSGRMousePayloadScrollsTranscript(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("[<64;57;23M"))
	if m.scrollOffset != 3 {
		t.Fatalf("scrollOffset after fragmented wheel up = %d, want 3", m.scrollOffset)
	}

	m, _ = update(t, m, keyRunes("[<65;72;19M"))
	if m.scrollOffset != 0 {
		t.Fatalf("scrollOffset after fragmented wheel down = %d, want 0", m.scrollOffset)
	}

	m, _ = update(t, m, keyRunes("[<80;57;23M"))
	if m.scrollOffset != 9 {
		t.Fatalf("scrollOffset after modified wheel up = %d, want 9", m.scrollOffset)
	}
}

func TestFragmentedSGRMousePayloadHandlesBatchedTails(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("[<64;57;23M[<64;58;23M"))

	if m.scrollOffset != 9 {
		t.Fatalf("scrollOffset after batched wheel tails = %d, want 9", m.scrollOffset)
	}
	if got := string(m.input); got != "" {
		t.Fatalf("input after batched mouse tails = %q, want empty", got)
	}
}

func TestSplitFragmentedSGRMousePayloadDoesNotEnterInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("draft")

	m, _ = update(t, m, keyEsc())
	for _, part := range []string{"[", "<64;", "57;23", "M"} {
		m, _ = update(t, m, keyRunes(part))
	}

	if got := string(m.input); got != "draft" {
		t.Fatalf("input after split mouse payload = %q, want draft", got)
	}
	if m.scrollOffset != 3 {
		t.Fatalf("scrollOffset after split wheel up = %d, want 3", m.scrollOffset)
	}
	if strings.Contains(m.status, "Esc again") || strings.Contains(m.status, "Press Esc again") {
		t.Fatalf("status = %q, want no esc prompt", m.status)
	}
}

func TestSplitFragmentedMousePayloadAfterEscTimeoutDoesNotEnterInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.scrollOffset = 1

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	m, _ = update(t, m, keyRunes("["))
	m, _ = update(t, m, keyRunes("<65;72;19M"))

	if got := string(m.input); got != "" {
		t.Fatalf("input after delayed split wheel down = %q, want empty", got)
	}
	if m.scrollOffset != 0 {
		t.Fatalf("scrollOffset after delayed split wheel down = %d, want 0", m.scrollOffset)
	}
}

func TestPartialMouseTailWithoutEscCompletesOrFlushes(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, cmd := update(t, m, keyRunes("["))
	if cmd == nil {
		t.Fatalf("partial mouse tail should schedule a timeout")
	}
	if got := string(m.input); got != "" {
		t.Fatalf("input while partial mouse tail is pending = %q, want empty", got)
	}
	m, _ = update(t, m, keyRunes("<64;57;23M"))
	if got := string(m.input); got != "" {
		t.Fatalf("input after completed partial mouse tail = %q, want empty", got)
	}
	if m.scrollOffset != 3 {
		t.Fatalf("scrollOffset after completed partial wheel up = %d, want 3", m.scrollOffset)
	}

	m, cmd = update(t, m, keyRunes("["))
	if cmd == nil {
		t.Fatalf("partial ordinary bracket should schedule a timeout")
	}
	m, _ = update(t, m, mouseTailTimeoutMsg{id: m.mouseTailID})
	if got := string(m.input); got != "[" {
		t.Fatalf("input after partial mouse timeout = %q, want [", got)
	}
}

func TestFragmentedMouseTailDoesNotSwallowOrdinaryInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("[not mouse]"))
	m, _ = update(t, m, keyRunes("[<64;57;23X"))

	if got := string(m.input); got != "[not mouse][<64;57;23X" {
		t.Fatalf("input after ordinary bracket text = %q", got)
	}
}

func TestPendingEscFragmentedSGRMousePayloadIsDropped(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("draft")

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, keyRunes("[<65;60;20M"))

	if got := string(m.input); got != "draft" {
		t.Fatalf("input after split mouse payload = %q, want draft", got)
	}
	if m.rewindSelector != nil {
		t.Fatalf("split mouse payload opened rewind selector")
	}
	if strings.Contains(m.status, "Press Esc again") {
		t.Fatalf("status = %q, want no esc prompt", m.status)
	}
}

func TestRunningSplitSGRMousePayloadDoesNotCancel(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true
	cancelled := false
	m.cancelRun = func() { cancelled = true }

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, keyRunes("[<64;43;17M"))

	if cancelled {
		t.Fatalf("split mouse payload called cancelRun")
	}
	if !m.running {
		t.Fatalf("running = false, want true")
	}
}

func TestEscTimeoutPromptsToClearNonEmptyInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("draft")

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})

	if got := string(m.input); got != "draft" {
		t.Fatalf("input after esc timeout = %q, want draft", got)
	}
	if !strings.Contains(m.status, "Esc again to clear") {
		t.Fatalf("status = %q, want clear prompt", m.status)
	}
}

func TestModelFileMentionSuggestionsAndTabCompletion(t *testing.T) {
	workDir := t.TempDir()
	writeTestFile(t, workDir, "internal/tui/model.go", "package tui\n")
	writeTestFile(t, workDir, "internal/session/session.go", "package session\n")
	writeTestFile(t, workDir, "README.md", "# Test\n")

	runner := newFakeRunner()
	runner.workDir = workDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyRunes("inspect @internal/t"))

	view := m.View()
	plainView := stripANSI(view)
	if !strings.Contains(plainView, "❯ @internal/tui/model.go") {
		t.Fatalf("view missing file mention suggestion:\n%s", view)
	}
	if !m.hasFileMentionMenu() {
		t.Fatalf("model should have file mention menu")
	}

	m, _ = update(t, m, keyTab())
	if got := string(m.input); got != "inspect @internal/tui/model.go " {
		t.Fatalf("input after file mention completion = %q", got)
	}
}

func TestModelFileMentionCompletionUsesTokenAtCursor(t *testing.T) {
	workDir := t.TempDir()
	writeTestFile(t, workDir, "internal/tui/model.go", "package tui\n")

	runner := newFakeRunner()
	runner.workDir = workDir
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("inspect @internal/tzz later")
	m.inputCursor = len([]rune("inspect @internal/t"))
	m.updateCompletions()

	if !m.hasFileMentionMenu() {
		t.Fatalf("model should have file mention menu at cursor token")
	}

	m, _ = update(t, m, keyTab())
	if got := string(m.input); got != "inspect @internal/tui/model.go later" {
		t.Fatalf("input after file mention completion at cursor = %q", got)
	}
	if got := m.inputCursor; got != len([]rune("inspect @internal/tui/model.go")) {
		t.Fatalf("cursor after file mention completion = %d", got)
	}
}

func TestModelFileMentionSelectionWithArrowKeys(t *testing.T) {
	workDir := t.TempDir()
	writeTestFile(t, workDir, "a.go", "package main\n")
	writeTestFile(t, workDir, "b.go", "package main\n")

	runner := newFakeRunner()
	runner.workDir = workDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyRunes("@"))

	if mention, ok := m.selectedFileMention(); !ok || mention.Path != "a.go" {
		t.Fatalf("initial selected file mention = %#v, %v; want a.go", mention, ok)
	}

	m, _ = update(t, m, keyDown())
	if mention, ok := m.selectedFileMention(); !ok || mention.Path != "b.go" {
		t.Fatalf("selected file mention after down = %#v, %v; want b.go", mention, ok)
	}

	m, _ = update(t, m, keyUp())
	if mention, ok := m.selectedFileMention(); !ok || mention.Path != "a.go" {
		t.Fatalf("selected file mention after up = %#v, %v; want a.go", mention, ok)
	}
}

func TestFileMentionsRespectRootGitignoreAndGitDirectory(t *testing.T) {
	workDir := t.TempDir()
	writeTestFile(t, workDir, ".gitignore", "*.log\nbuild/\n")
	writeTestFile(t, workDir, "src/main.go", "package main\n")
	writeTestFile(t, workDir, "debug.log", "ignored\n")
	writeTestFile(t, workDir, "build/out.txt", "ignored\n")
	writeTestFile(t, workDir, ".git/config", "ignored\n")

	mentions, err := discoverFileMentions(workDir)
	if err != nil {
		t.Fatalf("discoverFileMentions returned error: %v", err)
	}
	paths := fileMentionPaths(mentions)
	for _, want := range []string{".gitignore", "src/main.go"} {
		if !containsString(paths, want) {
			t.Fatalf("paths missing %q: %#v", want, paths)
		}
	}
	for _, forbidden := range []string{"debug.log", "build/out.txt", ".git/config"} {
		if containsString(paths, forbidden) {
			t.Fatalf("paths include ignored file %q: %#v", forbidden, paths)
		}
	}
}

func TestSlashDropdownSelectedRowsDoNotWrapDescriptions(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyRunes("/"))

	assertSlashDropdownLineCount(t, m.renderSlashSuggestions(72))

	m, _ = update(t, m, keyDown())
	m, _ = update(t, m, keyDown())
	assertSlashDropdownLineCount(t, m.renderSlashSuggestions(72))
}

func TestSlashDropdownUsesForegroundOnlySelection(t *testing.T) {
	if suggestionCommandStyle.GetForeground() != cAccentHi {
		t.Fatalf("non-selected slash command foreground = %q, want amber highlight", suggestionCommandStyle.GetForeground())
	}
	if suggestionDescriptionStyle.GetForeground() != cTextMuted {
		t.Fatalf("non-selected slash description foreground = %q, want muted amber", suggestionDescriptionStyle.GetForeground())
	}
	if suggestionSelectedStyle.GetForeground() != cWarn {
		t.Fatalf("selected slash command foreground = %q, want warning amber", suggestionSelectedStyle.GetForeground())
	}

	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyRunes("/"))
	rendered := m.renderSlashSuggestions(72)
	if strings.Contains(rendered, "\x1b[48;") {
		t.Fatalf("slash dropdown rendered background color escapes, want foreground colors only:\n%s", rendered)
	}
}

func TestFitLinePreservesANSIResetWhenTruncated(t *testing.T) {
	styled := markdownStyleRenderer.NewStyle().Foreground(cWarn).Render(strings.Repeat("x", 80))

	got := fitLine(styled, 12)

	if lipgloss.Width(got) > 12 {
		t.Fatalf("fitLine width = %d, want <= 12: %q", lipgloss.Width(got), got)
	}
	if !strings.HasSuffix(stripANSI(got), "...") {
		t.Fatalf("fitLine stripped output = %q, want truncation suffix", stripANSI(got))
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("fitLine truncated styled text without ANSI reset: %q", got)
	}
}

func TestHelpCommandRendersCommandsOnSeparateLines(t *testing.T) {
	rendered := renderEntry(entry{
		role:  "command",
		title: "Commands",
		body:  slashCommandHelp(),
		time:  time.Date(2026, 5, 17, 19, 18, 28, 0, time.Local),
	}, 100)
	plainRendered := stripANSI(rendered)
	lines := strings.Split(plainRendered, "\n")

	for _, forbidden := range []string{"SYSTEM commands", "###", "•", "19:18:28"} {
		if strings.Contains(plainRendered, forbidden) {
			t.Fatalf("rendered help contains noisy fragment %q:\n%s", forbidden, rendered)
		}
	}
	for _, command := range slashCommands {
		if !lineContainsAll(lines, command.Name, command.Description) {
			t.Fatalf("rendered help missing command row %q / %q:\n%s", command.Name, command.Description, rendered)
		}
	}
	if strings.Contains(plainRendered, "/session show current session paths /clear") {
		t.Fatalf("help commands collapsed onto one line:\n%s", rendered)
	}
	if strings.Count(plainRendered, "\n") < len(slashCommands) {
		t.Fatalf("help commands did not render across separate lines:\n%s", rendered)
	}
}

func TestSessionCommandRendersPlainAlignedRows(t *testing.T) {
	rendered := renderEntry(entry{
		role:  "command",
		title: "Session",
		body:  formatSessionRows("sess-1", "/tmp/sess-1", "/tmp/work", "fake-model"),
		time:  time.Date(2026, 5, 17, 19, 25, 30, 0, time.Local),
	}, 100)
	plainRendered := stripANSI(rendered)

	for _, forbidden := range []string{"SYSTEM session", "###", "• ID:", "`", "19:25:30"} {
		if strings.Contains(plainRendered, forbidden) {
			t.Fatalf("rendered session contains noisy fragment %q:\n%s", forbidden, rendered)
		}
	}
	for _, want := range []string{"Session", "ID       sess-1", "Dir      /tmp/sess-1", "Workdir  /tmp/work", "Model    fake-model"} {
		if !strings.Contains(plainRendered, want) {
			t.Fatalf("rendered session missing %q:\n%s", want, rendered)
		}
	}
}

func TestModelShiftTabTogglesPlanMode(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	if m.collaborationMode.PlanEnabled() {
		t.Fatalf("initial collaboration mode = %q, want Default", m.collaborationMode)
	}
	if !strings.Contains(m.View(), "plan mode off") || !strings.Contains(m.View(), "shift + tab to cycle") {
		t.Fatalf("plan mode off hint missing:\n%s", m.View())
	}

	m, _ = update(t, m, keyShiftTab())
	if !m.collaborationMode.PlanEnabled() || !runner.collaborationMode.PlanEnabled() {
		t.Fatalf("plan mode was not enabled: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}
	if m.status != "Plan mode enabled" {
		t.Fatalf("status = %q, want Plan mode enabled", m.status)
	}
	if !strings.Contains(m.View(), "plan mode on") {
		t.Fatalf("view missing plan on state:\n%s", m.View())
	}
	if strings.Contains(stripANSI(m.renderHeader(100)), "plan mode on") {
		t.Fatalf("plan mode should render in keybinds, not header:\n%s", m.renderHeader(100))
	}

	m, _ = update(t, m, keyShiftTab())
	if m.collaborationMode.PlanEnabled() || runner.collaborationMode.PlanEnabled() {
		t.Fatalf("plan mode was not disabled: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}
	if m.status != "Plan mode disabled" {
		t.Fatalf("status = %q, want Plan mode disabled", m.status)
	}
	if !strings.Contains(m.View(), "plan mode off") || !strings.Contains(m.View(), "shift + tab to cycle") {
		t.Fatalf("plan mode off hint missing after disabling:\n%s", m.View())
	}
}

func TestModelPlanCommandsAreIdempotent(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/plan"))
	m, _ = update(t, m, keyEnter())
	if !m.collaborationMode.PlanEnabled() || !runner.collaborationMode.PlanEnabled() {
		t.Fatalf("/plan did not enable Formal Plan: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}

	m, _ = update(t, m, keyRunes("/plan"))
	m, _ = update(t, m, keyEnter())
	if !m.collaborationMode.PlanEnabled() || !runner.collaborationMode.PlanEnabled() {
		t.Fatalf("repeated /plan changed Formal Plan: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}

	m, _ = update(t, m, keyRunes("/plan off"))
	m, _ = update(t, m, keyEnter())
	if m.collaborationMode.PlanEnabled() || runner.collaborationMode.PlanEnabled() {
		t.Fatalf("/plan off did not select Default: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}

	m, _ = update(t, m, keyRunes("/plan off"))
	m, _ = update(t, m, keyEnter())
	if m.collaborationMode.PlanEnabled() || runner.collaborationMode.PlanEnabled() {
		t.Fatalf("repeated /plan off changed Default: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}
}

func TestModelNewSessionResetsPlanModeSelection(t *testing.T) {
	runner := newFakeRunner()
	runner.collaborationMode = collaboration.ModeFormalPlan
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/new"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("/new command is nil")
	}
	m, _ = update(t, m, cmd())

	if m.collaborationMode.PlanEnabled() || runner.collaborationMode.PlanEnabled() {
		t.Fatalf("new session retained Formal Plan: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}
}

func TestModelTogglesPlanModeWhileRunningForNextRun(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true

	m, _ = update(t, m, keyShiftTab())
	if !m.collaborationMode.PlanEnabled() || !runner.collaborationMode.PlanEnabled() {
		t.Fatalf("plan mode was not enabled while running: model=%v runner=%v", m.collaborationMode, runner.collaborationMode)
	}
	if m.status != "Plan mode enabled for next run" {
		t.Fatalf("status = %q, want next-run plan mode status", m.status)
	}
}

func TestModelInputHistoryWithArrowKeys(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("first task"))
	m, cmd := update(t, m, keyEnter())
	m, _ = update(t, m, cmd())

	m, _ = update(t, m, keyRunes("second task"))
	m, cmd = update(t, m, keyEnter())
	m, _ = update(t, m, cmd())

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "second task" {
		t.Fatalf("first history recall = %q, want second task", got)
	}

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "first task" {
		t.Fatalf("second history recall = %q, want first task", got)
	}

	m, _ = update(t, m, keyDown())
	if got := string(m.input); got != "second task" {
		t.Fatalf("history next = %q, want second task", got)
	}

	m, _ = update(t, m, keyDown())
	if got := string(m.input); got != "" {
		t.Fatalf("history next past newest = %q, want empty draft", got)
	}
}

func TestModelSlashCommandInputHistoryWithArrowKeys(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/permissions"))
	m, _ = update(t, m, keyEnter())
	if m.permissionForm == nil {
		t.Fatal("/permissions did not open permissions form")
	}
	m, cmd := update(t, m, keyEsc())
	if cmd == nil {
		t.Fatal("permissions cancel command is nil")
	}
	m, _ = update(t, m, cmd())

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "/permissions" {
		t.Fatalf("slash command history recall = %q, want /permissions", got)
	}
}

func TestModelInputHistoryRestoresDraft(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("saved task"))
	m, cmd := update(t, m, keyEnter())
	m, _ = update(t, m, cmd())

	m, _ = update(t, m, keyRunes("draft"))
	m, _ = update(t, m, keyUp())
	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "saved task" {
		t.Fatalf("history recall = %q, want saved task", got)
	}

	m, _ = update(t, m, keyDown())
	if got := string(m.input); got != "draft" {
		t.Fatalf("restored draft = %q, want draft", got)
	}
}

func TestModelUpMovesWithinMultilineInputBeforeHistory(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.inputHistory = []string{"history"}

	m, _ = update(t, m, keyRunes("abcde"))
	m, _ = update(t, m, keyShiftEnter())
	m, _ = update(t, m, keyRunes("12345"))
	m, _ = update(t, m, keyUp())
	m, _ = update(t, m, keyRunes("X"))

	if got := string(m.input); got != "abcdeX\n12345" {
		t.Fatalf("input after moving up and typing = %q, want insertion on first line", got)
	}
}

func TestModelUpAtFirstVisibleLineJumpsHomeBeforeHistory(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.inputHistory = []string{"history"}
	m.input = []rune("abcdef")
	m.inputCursor = 3

	m, _ = update(t, m, keyUp())
	if got := m.inputCursor; got != 0 {
		t.Fatalf("cursor after first up = %d, want 0", got)
	}
	if got := string(m.input); got != "abcdef" {
		t.Fatalf("input after first up = %q, want unchanged draft", got)
	}

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "history" {
		t.Fatalf("input after second up = %q, want history", got)
	}
	if got := m.inputCursor; got != len([]rune("history")) {
		t.Fatalf("cursor after history recall = %d, want end", got)
	}
}

func TestModelDownAtLastVisibleLineJumpsEndBeforeHistory(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.inputHistory = []string{"history"}
	m.input = []rune("abcdef")
	m.inputCursor = 3

	m, _ = update(t, m, keyDown())
	if got := m.inputCursor; got != len(m.input) {
		t.Fatalf("cursor after first down = %d, want end", got)
	}
	if got := string(m.input); got != "abcdef" {
		t.Fatalf("input after first down = %q, want unchanged draft", got)
	}

	m.recallPreviousInput()
	m, _ = update(t, m, keyDown())
	if got := string(m.input); got != "abcdef" {
		t.Fatalf("input after history down = %q, want restored draft", got)
	}
	if got := m.inputCursor; got != 3 {
		t.Fatalf("cursor after draft restore = %d, want original draft cursor", got)
	}
}

func TestModelLongInputUpDownNavigateVisibleWrapBeforeHistory(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = minWidth
	m.inputHistory = []string{"history"}
	m.input = []rune(strings.Repeat("abcdefghij", 7))
	m.inputCursor = len(m.input)

	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != strings.Repeat("abcdefghij", 7) {
		t.Fatalf("input after wrap up = %q, want unchanged draft", got)
	}
	if m.inputCursor <= 0 || m.inputCursor >= len(m.input) {
		t.Fatalf("cursor after wrap up = %d, want inside input", m.inputCursor)
	}

	m, _ = update(t, m, keyDown())
	if got := m.inputCursor; got != len(m.input) {
		t.Fatalf("cursor after wrap down = %d, want end", got)
	}
}

func TestModelUpKeepsRenderedCursorColumnAcrossWrappedInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = minWidth
	m.input = []rune("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda")
	m.inputCursor = len(m.input)

	_, beforeCol, ok := renderedInputCursorPosition(m)
	if !ok {
		t.Fatalf("rendered input missing cursor before move:\n%s", m.renderInput(m.innerWidth()))
	}

	m, _ = update(t, m, keyUp())
	_, afterCol, ok := renderedInputCursorPosition(m)
	if !ok {
		t.Fatalf("rendered input missing cursor after move:\n%s", m.renderInput(m.innerWidth()))
	}
	if afterCol != beforeCol {
		t.Fatalf("cursor rendered column after up = %d, want %d\n%s", afterCol, beforeCol, stripANSI(m.renderInput(m.innerWidth())))
	}
}

func TestModelRenderInputHeightDoesNotChangeWhenCursorBlinks(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = minWidth
	m.input = []rune("alpha beta gamma delta epsilon zeta eta theta iota kappa lambda")
	m.inputCursor = len([]rune("alpha beta gamma delta epsilon zeta eta"))
	m.spinnerFrame = 0
	visibleHeight := lipgloss.Height(m.renderInput(m.innerWidth()))

	m.spinnerFrame = 1
	hiddenHeight := lipgloss.Height(m.renderInput(m.innerWidth()))
	if hiddenHeight != visibleHeight {
		t.Fatalf("input height changed across cursor blink: visible=%d hidden=%d\nvisible:\n%s\nhidden:\n%s", visibleHeight, hiddenHeight, stripANSI(m.renderInput(m.innerWidth())), stripANSI(m.renderInput(m.innerWidth())))
	}
}

func TestModelRenderInputSummarizesLongInputWithHeadAndTail(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = 80
	m.height = 20
	m.input = []rune(longPastedInputSample())
	m.inputCursor = len(m.input)

	rows := m.inputRenderRows()
	rendered := stripANSI(m.renderInput(m.innerWidth()))
	renderedRows := renderedInputContentRows(m)
	if got, total := len(renderedRows), len(rows); got >= total {
		t.Fatalf("rendered input rows = %d, want summarized below full row count %d\n%s", got, total, rendered)
	}
	if !strings.Contains(rendered, "Intro paragraph") || !strings.Contains(rendered, "final line") {
		t.Fatalf("summarized input should show both head and tail:\n%s", rendered)
	}
	if !strings.Contains(rendered, "hidden") {
		t.Fatalf("summarized input missing hidden-line marker:\n%s", rendered)
	}
}

func TestModelPastePreviewKeepsShortWindowViewWithinHeight(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 14})
	m, _ = update(t, m, keyPaste(numberedLines("line", 30)))

	if got := lipgloss.Height(m.View()); got > 14 {
		t.Fatalf("paste preview view height = %d, want <= actual window height 14", got)
	}
}

func TestModelViewKeepsLongInputWithinWindowWidth(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = 80
	m.height = 20
	m.input = []rune(longPastedInputSample())
	m.inputCursor = len(m.input)

	for i, line := range strings.Split(stripANSI(m.View()), "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("view line %d width = %d, want <= window width %d\n%q", i, got, m.width, line)
		}
	}
}

func TestModelViewWrapsReportedLongPastedInputWithinWindowWidth(t *testing.T) {
	for _, width := range []int{60, 80, 96, 120, 160, 220} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			runner := newFakeRunner()
			m := NewModel(context.Background(), runner, Config{})
			m.width = width
			m.height = 20
			m, _ = update(t, m, keyPaste(reportedLongPastedInputSample()))

			assertViewFitsTerminal(t, m)
		})
	}
}

func TestModelViewWrapsReportedLongPastedInputWithSidebarWithinWindowWidth(t *testing.T) {
	for _, width := range []int{120, 160, 220} {
		t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
			runner := newFakeRunner()
			m := NewModel(context.Background(), runner, Config{})
			m.width = width
			m.height = 20
			m.sidebarVisible = true
			m.sidebarDocuments = []sidebarDocument{
				{Title: "plan", Content: strings.Repeat("sidebar content ", 20)},
			}
			m, _ = update(t, m, keyPaste(reportedLongPastedInputSample()))

			assertViewFitsTerminal(t, m)
		})
	}
}

func TestModelLongPastedInputDoesNotRenderControlCharacters(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = 80
	m.height = 20
	m.input = []rune("first line\r\n\tindented with tab\r\nlast line")
	m.inputCursor = len(m.input)

	view := stripANSI(m.View())
	if strings.ContainsRune(view, '\r') {
		t.Fatalf("view rendered carriage return control character:\n%q", view)
	}
	if strings.ContainsRune(view, '\t') {
		t.Fatalf("view rendered tab control character:\n%q", view)
	}
}

func TestModelLongInputSummaryPreservesFormattedHeadAndTail(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = 72
	m.height = 20
	m.input = []rune(longPastedInputSample())
	m.inputCursor = len(m.input)
	m.spinnerFrame = 1

	rows := m.inputRenderRows()
	renderedRows := renderedInputContentRows(m)
	if len(renderedRows) >= len(rows) {
		t.Fatalf("long input rendered %d rows, want summarized below full %d rows", len(renderedRows), len(rows))
	}
	rendered := stripANSI(m.renderInput(m.innerWidth()))
	for _, want := range []string{
		"Intro paragraph",
		"final line",
		"hidden",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("summarized input missing %q:\n%s", want, rendered)
		}
	}
}

func TestModelHistoryRecallLongInputViewFitsAndShowsHeadAndTail(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m.inputHistory = []string{numberedLines("line", 30)}

	m, _ = update(t, m, keyUp())

	view := stripANSI(m.View())
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("view height = %d, want <= window height %d\n%s", got, m.height, view)
	}
	if !strings.Contains(view, "line 01") || !strings.Contains(view, "line 30") {
		t.Fatalf("history recall should show both head and tail:\n%s", view)
	}
	if !strings.Contains(view, "hidden") {
		t.Fatalf("history recall missing hidden-line marker:\n%s", view)
	}
}

func TestModelLongInputHeightDoesNotChangeWhenCursorBlinks(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = 80
	m.height = 20
	m.input = []rune(numberedLines("line", 30))
	m.inputCursor = len(m.input)
	m.spinnerFrame = 0
	visibleHeight := lipgloss.Height(m.renderInput(m.innerWidth()))

	m.spinnerFrame = 1
	hiddenHeight := lipgloss.Height(m.renderInput(m.innerWidth()))
	if hiddenHeight != visibleHeight {
		t.Fatalf("long input height changed across cursor blink: visible=%d hidden=%d", visibleHeight, hiddenHeight)
	}
}

func TestModelMultilinePasteRendersPreviewButKeepsFullInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyPaste("a\nb\nc"))

	if got := string(m.input); got != "a\nb\nc" {
		t.Fatalf("input = %q, want full pasted text", got)
	}
	rendered := stripANSI(m.renderInput(m.innerWidth()))
	if !strings.Contains(rendered, "[pasted text #1 +2 lines]") {
		t.Fatalf("rendered input missing paste preview:\n%s", rendered)
	}
	if strings.Contains(rendered, "a\n") || strings.Contains(rendered, "\nb") || strings.Contains(rendered, "\nc") {
		t.Fatalf("rendered input expanded pasted text:\n%s", rendered)
	}
}

func TestModelPasteWithCarriageReturnsRendersPreview(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyPaste("a\rb\rc"))

	if got := string(m.input); got != "a\nb\nc" {
		t.Fatalf("input = %q, want normalized pasted text", got)
	}
	rendered := stripANSI(m.renderInput(m.innerWidth()))
	if !strings.Contains(rendered, "[pasted text #1 +2 lines]") {
		t.Fatalf("rendered input missing paste preview for CR paste:\n%s", rendered)
	}
}

func TestModelSubmittingCarriageReturnPasteRendersUserEntryWithLineBreaks(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyPaste("first\rsecond\rthird"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("enter did not submit CR paste input")
	}

	rendered := stripANSI(renderEntry(m.entries[len(m.entries)-1], m.innerWidth()))
	lines := strings.Split(rendered, "\n")
	for _, want := range []string{"first", "second", "third"} {
		if !lineContainsAll(lines, want) {
			t.Fatalf("submitted CR paste missing rendered line %q:\n%q", want, rendered)
		}
	}
	if strings.ContainsRune(rendered, '\r') {
		t.Fatalf("submitted CR paste rendered carriage return control character:\n%q", rendered)
	}
}

func TestModelSubmittingPastePreviewSendsFullText(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyPaste("a\nb\nc"))
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatalf("enter did not submit paste preview input")
	}
	m, _ = update(t, m, cmd())

	if len(runner.runs) != 1 || runner.runs[0] != "a\nb\nc" {
		t.Fatalf("runs = %#v, want full pasted text", runner.runs)
	}
	if got := string(m.input); got != "" {
		t.Fatalf("input after submit = %q, want cleared", got)
	}
}

func TestModelEditingAfterPastePreviewRestoresNormalRendering(t *testing.T) {
	t.Run("ordinary character", func(t *testing.T) {
		runner := newFakeRunner()
		m := NewModel(context.Background(), runner, Config{})

		m, _ = update(t, m, keyPaste("a\nb\nc"))
		m, _ = update(t, m, keyRunes("d"))

		rendered := stripANSI(m.renderInput(m.innerWidth()))
		if strings.Contains(rendered, "[pasted text") {
			t.Fatalf("rendered input kept paste preview after edit:\n%s", rendered)
		}
		if !strings.Contains(rendered, "a") || !strings.Contains(rendered, "b") || !strings.Contains(rendered, "cd") {
			t.Fatalf("rendered input missing normal edited text:\n%s", rendered)
		}
	})

	t.Run("backspace", func(t *testing.T) {
		runner := newFakeRunner()
		m := NewModel(context.Background(), runner, Config{})

		m, _ = update(t, m, keyPaste("a\nb\nc"))
		m, _ = update(t, m, keyBackspace())

		rendered := stripANSI(m.renderInput(m.innerWidth()))
		if strings.Contains(rendered, "[pasted text") {
			t.Fatalf("rendered input kept paste preview after backspace:\n%s", rendered)
		}
		if got := string(m.input); got != "a\nb\n" {
			t.Fatalf("input after backspace = %q, want edited full text", got)
		}
	})
}

func TestModelHistoryRecallDoesNotUsePastePreview(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = 80
	m.height = 20
	m.inputHistory = []string{numberedLines("line", 30)}

	m, _ = update(t, m, keyUp())

	rendered := stripANSI(m.renderInput(m.innerWidth()))
	if strings.Contains(rendered, "[pasted text") {
		t.Fatalf("history recall rendered paste preview:\n%s", rendered)
	}
	if !strings.Contains(rendered, "line 01") || !strings.Contains(rendered, "line 30") {
		t.Fatalf("history recall missing summarized multiline head/tail:\n%s", rendered)
	}
}

func TestModelPastePreviewLineFitsWindowWidth(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = minWidth

	m, _ = update(t, m, keyPaste(numberedLines("line", 30)))

	rendered := stripANSI(m.renderInput(m.innerWidth()))
	if !strings.Contains(rendered, "[pasted text #1 +29 lines]") {
		t.Fatalf("rendered input missing paste preview:\n%s", rendered)
	}
	for i, line := range strings.Split(rendered, "\n") {
		if got := lipgloss.Width(line); got > m.innerWidth() {
			t.Fatalf("rendered input line %d width = %d, want <= inner width %d\n%q", i, got, m.innerWidth(), line)
		}
	}
}

func TestModelInputCursorRemainsVisibleAcrossSpinnerFrames(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.width = 80
	m.input = []rune("abc")
	m.inputCursor = len(m.input)

	m.spinnerFrame = 0
	if !strings.Contains(stripANSI(m.renderInput(m.innerWidth())), "abc▌") {
		t.Fatalf("cursor missing on even frame:\n%s", m.renderInput(m.innerWidth()))
	}

	m.spinnerFrame = 1
	if !strings.Contains(stripANSI(m.renderInput(m.innerWidth())), "abc▌") {
		t.Fatalf("cursor should remain visible on odd frame:\n%s", m.renderInput(m.innerWidth()))
	}
}

func TestModelCursorEditingKeysModifyAtCursor(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("abcd")
	m.inputCursor = 2

	m, _ = update(t, m, keyRunes("X"))
	if got := string(m.input); got != "abXcd" {
		t.Fatalf("input after insert = %q, want abXcd", got)
	}

	m, _ = update(t, m, keyLeft())
	m, _ = update(t, m, keyDelete())
	if got := string(m.input); got != "abcd" {
		t.Fatalf("input after delete = %q, want abcd", got)
	}

	m, _ = update(t, m, keyBackspace())
	if got := string(m.input); got != "acd" {
		t.Fatalf("input after backspace = %q, want acd", got)
	}

	m, _ = update(t, m, keyRight())
	m, _ = update(t, m, keyRunes("Y"))
	m, _ = update(t, m, keyHome())
	m, _ = update(t, m, keyRunes(">"))
	m, _ = update(t, m, keyEnd())
	m, _ = update(t, m, keyRunes("<"))
	if got := string(m.input); got != ">acYd<" {
		t.Fatalf("input after home/end insertions = %q, want >acYd<", got)
	}
}

func TestModelInputHistoryRestoresDraftCursor(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.inputHistory = []string{"saved task"}
	m.input = []rune("draft")
	m.inputCursor = 2

	m, _ = update(t, m, keyUp())
	m, _ = update(t, m, keyUp())
	m, _ = update(t, m, keyDown())
	if got := string(m.input); got != "draft" {
		t.Fatalf("restored draft = %q, want draft", got)
	}
	if got := m.inputCursor; got != 2 {
		t.Fatalf("restored draft cursor = %d, want 2", got)
	}
}

func TestModelViewRendersCursorAtInputCursor(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("abc")
	m.inputCursor = 1

	view := m.View()
	plainView := stripANSI(view)
	if !strings.Contains(plainView, "abc") {
		t.Fatalf("view missing cursor at middle position:\n%s", view)
	}
	if strings.Contains(plainView, "a▌c") || strings.Contains(plainView, "a▌bc") || strings.Contains(plainView, "abc▌") {
		t.Fatalf("view replaced text with cursor glyph:\n%s", view)
	}
}

func TestModelCursorOverlaysNextRuneWithoutChangingBackspaceSemantics(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("abcd")
	m.inputCursor = len(m.input)

	endView := stripANSI(m.renderInput(m.innerWidth()))
	if !strings.Contains(endView, "abcd▌") {
		t.Fatalf("cursor at end should render after final rune:\n%s", endView)
	}

	m, _ = update(t, m, keyLeft())
	middleView := stripANSI(m.renderInput(m.innerWidth()))
	if !strings.Contains(middleView, "abcd") {
		t.Fatalf("cursor after left should keep d visible:\n%s", middleView)
	}
	if strings.Contains(middleView, "abc▌") || strings.Contains(middleView, "abc▌d") {
		t.Fatalf("cursor after left replaced or shifted d instead of highlighting it:\n%s", middleView)
	}

	m, _ = update(t, m, keyBackspace())
	if got := string(m.input); got != "abd" {
		t.Fatalf("input after backspace = %q, want abd", got)
	}
}

func TestModelExitSlashCommandQuits(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/exit"))
	_, cmd := update(t, m, keyEnter())
	assertQuitCommand(t, cmd)
}

func TestModelCtrlCRequiresSecondPressWithinTwoSeconds(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return current }

	m, cmd := update(t, m, keyCtrlC())
	if cmd != nil {
		t.Fatalf("first ctrl+c returned quit command")
	}
	if !strings.Contains(m.status, "again within 2s") {
		t.Fatalf("status = %q, want confirmation prompt", m.status)
	}

	current = current.Add(time.Second)
	_, cmd = update(t, m, keyCtrlC())
	assertQuitCommand(t, cmd)
}

func TestModelCtrlCConfirmationExpires(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return current }

	m, cmd := update(t, m, keyCtrlC())
	if cmd != nil {
		t.Fatalf("first ctrl+c returned quit command")
	}

	current = current.Add(3 * time.Second)
	_, cmd = update(t, m, keyCtrlC())
	if cmd != nil {
		t.Fatalf("second ctrl+c after timeout returned quit command")
	}
}

func TestModelQueuesInputWhileRunIsActiveAndCancelsCurrentRun(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("long task"))
	m, _ = update(t, m, keyEnter())
	if !m.running {
		t.Fatalf("model running = false, want true")
	}

	m, _ = update(t, m, keyRunes("queued follow-up"))
	m, _ = update(t, m, keyEnter())
	if got := string(m.input); got != "" {
		t.Fatalf("input after queueing = %q, want empty", got)
	}
	if len(m.queuedPrompts) != 1 || m.queuedPrompts[0].text != "queued follow-up" {
		t.Fatalf("queuedPrompts = %#v, want queued follow-up", m.queuedPrompts)
	}
	if !strings.Contains(m.status, "Queued 1 message") {
		t.Fatalf("status = %q, want queued message status", m.status)
	}
	if !strings.Contains(stripANSI(m.View()), "1. queued follow-up") {
		t.Fatalf("view missing queued prompt preview:\n%s", m.View())
	}

	m, _ = update(t, m, keyEsc())
	m, _ = update(t, m, pendingEscTimeoutMsg{id: m.pendingEscID})
	if !strings.Contains(m.status, "Cancel") {
		t.Fatalf("status = %q, want cancel status", m.status)
	}
	if !entriesContain(m.entries, "system", "cancellation requested") {
		t.Fatalf("entries missing cancel notice: %#v", m.entries)
	}
}

func TestModelQueuedPromptKeepsCollaborationModeSelectedAtSubmission(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true

	m, _ = update(t, m, keyShiftTab())
	if m.collaborationMode != collaboration.ModeFormalPlan {
		t.Fatalf("collaboration mode = %q, want Formal Plan before queueing", m.collaborationMode)
	}
	m, _ = update(t, m, keyRunes("queued formal task"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("queueing prompt returned command")
	}

	// The active run may reset the selected mode before this queued submission
	// starts. The queued submission must still use the mode selected when Enter
	// was pressed.
	m.collaborationMode = collaboration.ModeDefault
	runner.SetCollaborationMode(collaboration.ModeDefault)
	m, queuedCmd := update(t, m, runFinishedMsg{result: &engine.RunResult{RunID: "active-run"}})
	if queuedCmd == nil {
		t.Fatal("active run completion did not start queued prompt")
	}
	_, _ = update(t, m, queuedCmd())

	if len(runner.runModes) != 1 || runner.runModes[0] != collaboration.ModeFormalPlan {
		t.Fatalf("run modes = %#v, want queued prompt frozen to Formal Plan", runner.runModes)
	}
}

func TestModelPromptKeepsCollaborationModeBeforeRunCommandStarts(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyShiftTab())
	m, _ = update(t, m, keyRunes("formal task"))
	_, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("submitting prompt did not return run command")
	}

	runner.SetCollaborationMode(collaboration.ModeDefault)
	_ = cmd()
	if len(runner.runModes) != 1 || runner.runModes[0] != collaboration.ModeFormalPlan {
		t.Fatalf("run modes = %#v, want submitted prompt frozen to Formal Plan", runner.runModes)
	}
}

func TestModelRunningNoticeShowsQueuedPromptPreviews(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 5, 0, time.UTC)
	m.now = func() time.Time { return current }
	m.running = true
	m.runStartedAt = current.Add(-5 * time.Second)
	m.queuedPrompts = testQueuedPrompts(
		"second task",
		"third task\nwith newline",
		strings.Repeat("long ", 40),
		"fourth task",
	)

	notice := stripANSI(m.renderRunningNotice(72))
	for _, want := range []string{
		"✦ working... 5s • esc to interrupt",
		"1. second task",
		"2. third task with newline",
		"3. long long",
		"... 1 more",
	} {
		if !strings.Contains(notice, want) {
			t.Fatalf("notice missing queued preview %q:\n%s", want, notice)
		}
	}
	if strings.Contains(notice, "4. fourth task") {
		t.Fatalf("notice should cap visible queued prompts:\n%s", notice)
	}
	if strings.Contains(notice, "elapsed") {
		t.Fatalf("notice should not include elapsed label:\n%s", notice)
	}
}

func TestModelRunsQueuedPromptsInFIFOOrder(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("first task"))
	m, firstCmd := update(t, m, keyEnter())
	if firstCmd == nil {
		t.Fatalf("first task command is nil")
	}

	m, _ = update(t, m, keyRunes("second task"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("queueing second task returned command, want nil")
	}
	m, _ = update(t, m, keyRunes("third task"))
	m, cmd = update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("queueing third task returned command, want nil")
	}
	if got := strings.Join(queuedPromptTexts(m.queuedPrompts), ","); got != "second task,third task" {
		t.Fatalf("queuedPrompts = %#v, want FIFO second/third", m.queuedPrompts)
	}

	m, secondCmd := update(t, m, firstCmd())
	if secondCmd == nil {
		t.Fatalf("second queued task command is nil")
	}
	if !m.running {
		t.Fatalf("model should be running the second queued task")
	}
	if got := strings.Join(runner.runs, ","); got != "first task" {
		t.Fatalf("runs after first completion = %#v, want first task only", runner.runs)
	}
	if got := strings.Join(queuedPromptTexts(m.queuedPrompts), ","); got != "third task" {
		t.Fatalf("queuedPrompts after starting second = %#v, want third task", m.queuedPrompts)
	}
	if !entriesContain(m.entries, "user", "second task") {
		t.Fatalf("entries missing second task user message after it starts: %#v", m.entries)
	}

	m, thirdCmd := update(t, m, secondCmd())
	if thirdCmd == nil {
		t.Fatalf("third queued task command is nil")
	}
	if got := strings.Join(runner.runs, ","); got != "first task,second task" {
		t.Fatalf("runs after second completion = %#v, want first,second", runner.runs)
	}
	if len(m.queuedPrompts) != 0 {
		t.Fatalf("queuedPrompts after starting third = %#v, want empty", m.queuedPrompts)
	}

	m, finalCmd := update(t, m, thirdCmd())
	if finalCmd != nil {
		t.Fatalf("final queued task returned unexpected command")
	}
	if m.running {
		t.Fatalf("model running = true after final queued run")
	}
	if got := strings.Join(runner.runs, ","); got != "first task,second task,third task" {
		t.Fatalf("runs after final completion = %#v, want first,second,third", runner.runs)
	}
	for _, prompt := range []string{"first task", "second task", "third task"} {
		if !entriesContain(m.entries, "user", prompt) {
			t.Fatalf("entries missing user prompt %q: %#v", prompt, m.entries)
		}
		if !entriesContain(m.entries, "assistant", "answer: "+prompt) {
			t.Fatalf("entries missing assistant response for %q: %#v", prompt, m.entries)
		}
	}
}

func TestModelCommandSwitchesModelWhenIdle(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, cmd := update(t, m, keyRunes("/model test-model"))
	if cmd != nil {
		t.Fatalf("typing model command returned command")
	}
	m, cmd = update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/model returned command, want nil")
	}

	if runner.Model() != "test-model" {
		t.Fatalf("runner model = %q, want test-model", runner.Model())
	}
	if m.modelName != "test-model" {
		t.Fatalf("modelName = %q, want test-model", m.modelName)
	}
	if !entriesContain(m.entries, "command", "Switched model to test-model") {
		t.Fatalf("entries missing model switch notice: %#v", m.entries)
	}
	if !strings.Contains(stripANSI(m.renderStatusBar(100)), "test-model") {
		t.Fatalf("status bar missing switched model:\n%s", m.renderStatusBar(100))
	}
}

func TestModelCommandWithoutArgumentShowsUsage(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/model"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/model returned command, want nil")
	}

	if runner.Model() != "fake-model" {
		t.Fatalf("runner model changed to %q", runner.Model())
	}
	if !entriesContain(m.entries, "command", "Usage: /model <model_name>") {
		t.Fatalf("entries missing model usage: %#v", m.entries)
	}
}

func TestModelCommandQueuedBetweenPromptsRunsInFIFOOrder(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("first task"))
	m, firstCmd := update(t, m, keyEnter())
	if firstCmd == nil {
		t.Fatalf("first task command is nil")
	}
	m, _ = update(t, m, keyRunes("second task"))
	m, _ = update(t, m, keyEnter())
	m, _ = update(t, m, keyRunes("/model next-model"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("queued model command returned command")
	}
	m, _ = update(t, m, keyRunes("third task"))
	m, _ = update(t, m, keyEnter())

	if runner.Model() != "fake-model" {
		t.Fatalf("model switched while run active: %q", runner.Model())
	}
	if got := strings.Join(queuedPromptTexts(m.queuedPrompts), ","); got != "second task,/model next-model,third task" {
		t.Fatalf("queuedPrompts = %#v, want prompt/model/prompt", m.queuedPrompts)
	}

	m, secondCmd := update(t, m, firstCmd())
	if secondCmd == nil {
		t.Fatalf("second task command is nil")
	}
	if runner.Model() != "fake-model" {
		t.Fatalf("model switched before second prompt: %q", runner.Model())
	}

	m, thirdCmd := update(t, m, secondCmd())
	if thirdCmd == nil {
		t.Fatalf("third task command is nil")
	}
	if runner.Model() != "next-model" {
		t.Fatalf("model after queued command = %q, want next-model", runner.Model())
	}
	if !entriesContain(m.entries, "command", "Switched model to next-model") {
		t.Fatalf("entries missing queued model switch: %#v", m.entries)
	}

	m, finalCmd := update(t, m, thirdCmd())
	if finalCmd != nil {
		t.Fatalf("final queued task returned unexpected command")
	}
	if got := strings.Join(runner.runs, ","); got != "first task,second task,third task" {
		t.Fatalf("runs = %#v, want all prompts in order", runner.runs)
	}
}

func TestInvalidQueuedModelCommandContinuesQueue(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("first task"))
	m, firstCmd := update(t, m, keyEnter())
	m, _ = update(t, m, keyRunes("/model too many args"))
	m, _ = update(t, m, keyEnter())
	m, _ = update(t, m, keyRunes("second task"))
	m, _ = update(t, m, keyEnter())

	m, secondCmd := update(t, m, firstCmd())
	if secondCmd == nil {
		t.Fatalf("second task command is nil")
	}
	if !entriesContain(m.entries, "error", "Usage: /model <model_name>") {
		t.Fatalf("entries missing invalid queued model error: %#v", m.entries)
	}
	m, _ = update(t, m, secondCmd())
	if got := strings.Join(runner.runs, ","); got != "first task,second task" {
		t.Fatalf("runs = %#v, want queue to continue after invalid model command", runner.runs)
	}
}

func TestModelRunsQueuedPromptAfterRunError(t *testing.T) {
	runner := newFakeRunner()
	runner.runErrs = []error{errors.New("first run failed")}
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("first task"))
	m, firstCmd := update(t, m, keyEnter())
	m, _ = update(t, m, keyRunes("second task"))
	m, _ = update(t, m, keyEnter())

	m, secondCmd := update(t, m, firstCmd())
	if secondCmd == nil {
		t.Fatalf("queued task should start after failed run")
	}
	if !m.running {
		t.Fatalf("model should run queued prompt after first run fails")
	}
	if !entriesContain(m.entries, "error", "first run failed") {
		t.Fatalf("entries missing first run error: %#v", m.entries)
	}

	m, cmd := update(t, m, secondCmd())
	if cmd != nil {
		t.Fatalf("second task returned unexpected follow-up command")
	}
	if m.running {
		t.Fatalf("model running = true after queued prompt finishes")
	}
	if got := strings.Join(runner.runs, ","); got != "first task,second task" {
		t.Fatalf("runs = %#v, want failed first task then queued second task", runner.runs)
	}
	if !entriesContain(m.entries, "assistant", "answer: second task") {
		t.Fatalf("entries missing queued task assistant response: %#v", m.entries)
	}
}

func TestModelRunError(t *testing.T) {
	runner := newFakeRunner()
	runner.runErr = errors.New("model unavailable")
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("fail"))
	m, cmd := update(t, m, keyEnter())
	m, _ = update(t, m, cmd())

	if m.running {
		t.Fatalf("model running = true after failed run")
	}
	if m.status != "Run failed" {
		t.Fatalf("status = %q, want Run failed", m.status)
	}
	if !entriesContain(m.entries, "error", "model unavailable") {
		t.Fatalf("entries missing error: %#v", m.entries)
	}
	if got := countEntriesContaining(m.entries, "error", "model unavailable"); got != 1 {
		t.Fatalf("error entries = %d, want 1; entries = %#v", got, m.entries)
	}
}

func TestModelViewContainsSessionAndInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{Model: "fake-model"})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()
	plainView := stripANSI(view)
	for _, want := range []string{"SYSTEM session", "Interactive session started. Type /help for commands.", "fake-model", "work", "git -", "Context 7%", "> ▌ ask anything, or /help for commands"} {
		if !strings.Contains(plainView, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Index(plainView, "SYSTEM session") > strings.Index(plainView, "ask anything, or /help for commands") {
		t.Fatalf("session notice should render above the input box:\n%s", view)
	}
	if !strings.Contains(plainView, "plan mode off") || !strings.Contains(plainView, "shift + tab to cycle") {
		t.Fatalf("view missing plan mode hint:\n%s", view)
	}
}

func TestModelWideViewRendersSidebarDocuments(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "PLAN.md", "stale project plan")
	writeTestFile(t, workDir, "TODO.md", "stale project todo")
	writeTestFile(t, sessionDir, "PLAN.md", "- Build right sidebar")
	writeTestFile(t, sessionDir, "TODO.md", "- [ ] Add tests")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	// The Memory panel now reflects the cross-session persistent memory index.
	runner.memoryIndex = "Remember the repo conventions."
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	plainView := stripANSI(m.View())
	for _, want := range []string{
		"MEMORY",
		"Remember the repo",
		"conventions.",
		"PLAN",
		"Build right sidebar",
		"TODO",
		"Add tests",
	} {
		if !strings.Contains(plainView, want) {
			t.Fatalf("wide view missing sidebar content %q:\n%s", want, plainView)
		}
	}
	if strings.Contains(plainView, "stale project plan") || strings.Contains(plainView, "stale project todo") {
		t.Fatalf("sidebar should use session plan/todo instead of project files:\n%s", plainView)
	}
}

func TestSidebarBoxHeightsSplitDocumentAreaEvenly(t *testing.T) {
	heights := sidebarBoxHeights(22, 3)
	if len(heights) != 3 {
		t.Fatalf("sidebarBoxHeights len = %d, want 3: %#v", len(heights), heights)
	}

	total := 0
	minHeight := heights[0]
	maxHeight := heights[0]
	for _, height := range heights {
		total += height
		minHeight = min(minHeight, height)
		maxHeight = max(maxHeight, height)
	}
	if total != 22 {
		t.Fatalf("sidebarBoxHeights total = %d, want 22: %#v", total, heights)
	}
	if maxHeight-minHeight > 1 {
		t.Fatalf("sidebarBoxHeights should differ by at most one row: %#v", heights)
	}
}

func TestModelWideViewSidebarLongPlanStartsAtTop(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 24))
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	plainView := stripANSI(m.View())
	if !strings.Contains(plainView, "plan line 01") {
		t.Fatalf("default sidebar plan view should show top content:\n%s", plainView)
	}
	if strings.Contains(plainView, "plan line 24") {
		t.Fatalf("default sidebar plan view should hide later content:\n%s", plainView)
	}
}

func TestModelMouseWheelScrollsSidebarPlan(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 24))
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	x, y := sidebarPoint(t, m, 1)
	m, _ = update(t, m, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown})
	if m.sidebarScrollOffsets[1] != 3 {
		t.Fatalf("plan sidebar offset after wheel down = %d, want 3", m.sidebarScrollOffsets[1])
	}
	if m.sidebarScrollOffsets[0] != 0 || m.sidebarScrollOffsets[2] != 0 {
		t.Fatalf("scrolling plan should not affect other sidebar offsets: %#v", m.sidebarScrollOffsets)
	}

	plainView := stripANSI(m.View())
	if strings.Contains(plainView, "plan line 01") {
		t.Fatalf("scrolled sidebar plan should hide the first line:\n%s", plainView)
	}
	if !strings.Contains(plainView, "plan line 04") {
		t.Fatalf("scrolled sidebar plan should show later content:\n%s", plainView)
	}
}

func TestSidebarDragSelectionCopiesSingleLine(t *testing.T) {
	m, copied := selectableSidebarModel(t, "alpha beta gamma")
	line, col := findSidebarText(t, m, "alpha beta gamma")
	x, y := sidebarSelectionPoint(t, m, line, col)

	m, _ = update(t, m, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, _ = update(t, m, tea.MouseMsg{X: x + len("alpha beta"), Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, _ = update(t, m, tea.MouseMsg{X: x + len("alpha beta"), Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	if *copied != "alpha beta" {
		t.Fatalf("copied = %q, want alpha beta", *copied)
	}
}

func TestSidebarDragSelectionCopiesMultipleLinesAndReverse(t *testing.T) {
	m, copied := selectableSidebarModel(t, "alpha beta\ngamma delta")
	firstLine, firstCol := findSidebarText(t, m, "alpha beta")
	secondLine, secondCol := findSidebarText(t, m, "gamma delta")
	startX, startY := sidebarSelectionPoint(t, m, secondLine, secondCol+len("gamma"))
	endX, endY := sidebarSelectionPoint(t, m, firstLine, firstCol+len("alpha "))

	m, _ = update(t, m, tea.MouseMsg{X: startX, Y: startY, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	m, _ = update(t, m, tea.MouseMsg{X: endX, Y: endY, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion})
	m, _ = update(t, m, tea.MouseMsg{X: endX, Y: endY, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease})

	want := "beta\ngamma"
	if *copied != want {
		t.Fatalf("copied = %q, want %q", *copied, want)
	}
}

func TestSidebarSelectionHighlightDoesNotHighlightTranscript(t *testing.T) {
	m, _ := selectableSidebarModel(t, "alpha beta")
	m.entries = nil
	m.appendEntry("assistant", "", "alpha beta", false)
	line, col := findSidebarText(t, m, "alpha beta")
	m.selection = selectionState{
		anchor: selectionPoint{line: line, col: col + len("alpha ")},
		focus:  selectionPoint{line: line, col: col + len("alpha beta")},
		active: true,
		area:   selectionAreaSidebar,
	}

	renderedSidebar := m.renderSidebar(m.sidebarWidth(), m.transcriptHeight())
	if !strings.Contains(renderedSidebar, selectionStyle.Render("beta")) {
		t.Fatalf("rendered sidebar does not contain highlighted selection:\n%s", renderedSidebar)
	}
	renderedTranscript := m.renderBody(m.chatWidth(), m.transcriptHeight())
	unselected := m
	unselected.clearSelection()
	if renderedTranscript != unselected.renderBody(unselected.chatWidth(), unselected.transcriptHeight()) {
		t.Fatalf("transcript changed for sidebar selection:\n%s", renderedTranscript)
	}
}

func TestFragmentedMousePayloadScrollsSidebarPlan(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 24))
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	x, y := sidebarPoint(t, m, 1)
	payload := fmt.Sprintf("[<65;%d;%dM", x+1, y+1)
	m, _ = update(t, m, keyRunes(payload))

	if m.sidebarScrollOffsets[1] != 3 {
		t.Fatalf("plan sidebar offset after fragmented wheel down = %d, want 3", m.sidebarScrollOffsets[1])
	}
	if got := string(m.input); got != "" {
		t.Fatalf("input after fragmented sidebar wheel = %q, want empty", got)
	}
}

func TestModelKeyboardFocusScrollsSidebarPlan(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", numberedLines("memory line", 12))
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 24))
	writeTestFile(t, sessionDir, "TODO.md", numberedLines("todo line", 12))

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	m, _ = update(t, m, keyCtrlF())
	if !m.sidebarFocused {
		t.Fatalf("ctrl+f did not focus sidebar")
	}
	if m.sidebarFocusIndex != 1 {
		t.Fatalf("initial sidebar focus index = %d, want Plan index 1", m.sidebarFocusIndex)
	}

	m, _ = update(t, m, keyDown())
	if m.sidebarScrollOffsets[1] != 1 {
		t.Fatalf("plan sidebar offset after down = %d, want 1", m.sidebarScrollOffsets[1])
	}
	if m.scrollOffset != 0 {
		t.Fatalf("sidebar focus down changed transcript scrollOffset = %d, want 0", m.scrollOffset)
	}
	if m.sidebarScrollOffsets[0] != 0 || m.sidebarScrollOffsets[2] != 0 {
		t.Fatalf("scrolling focused plan should not affect other sidebar offsets: %#v", m.sidebarScrollOffsets)
	}

	plainView := stripANSI(m.View())
	if strings.Contains(plainView, "plan line 01") {
		t.Fatalf("focused sidebar scroll should hide the first plan line:\n%s", plainView)
	}
	if !strings.Contains(plainView, "plan line 03") {
		t.Fatalf("focused sidebar scroll should show later plan content:\n%s", plainView)
	}
}

func TestModelKeyboardSidebarFocusSwitchAndBounds(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", numberedLines("memory line", 20))
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 20))
	writeTestFile(t, sessionDir, "TODO.md", numberedLines("todo line", 20))

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})
	m, _ = update(t, m, keyCtrlF())

	m, _ = update(t, m, keyTab())
	if m.sidebarFocusIndex != 2 {
		t.Fatalf("tab sidebar focus index = %d, want Todo index 2", m.sidebarFocusIndex)
	}
	m, _ = update(t, m, keyShiftTab())
	if m.sidebarFocusIndex != 1 {
		t.Fatalf("shift+tab sidebar focus index = %d, want Plan index 1", m.sidebarFocusIndex)
	}
	m, _ = update(t, m, keyRunes("1"))
	if m.sidebarFocusIndex != 0 {
		t.Fatalf("numeric sidebar focus index = %d, want Memory index 0", m.sidebarFocusIndex)
	}

	m, _ = update(t, m, keyEnd())
	maxOffset := m.maxFocusedSidebarOffset()
	if m.sidebarScrollOffsets[0] != maxOffset {
		t.Fatalf("end sidebar offset = %d, want %d", m.sidebarScrollOffsets[0], maxOffset)
	}
	m, _ = update(t, m, keyPgDown())
	if m.sidebarScrollOffsets[0] != maxOffset {
		t.Fatalf("pgdown should clamp at max offset = %d, want %d", m.sidebarScrollOffsets[0], maxOffset)
	}
	m, _ = update(t, m, keyHome())
	if m.sidebarScrollOffsets[0] != 0 {
		t.Fatalf("home sidebar offset = %d, want 0", m.sidebarScrollOffsets[0])
	}
	m, _ = update(t, m, keyPgUp())
	if m.sidebarScrollOffsets[0] != 0 {
		t.Fatalf("pgup should clamp at top offset = %d, want 0", m.sidebarScrollOffsets[0])
	}
}

func TestModelSidebarFocusUnavailableAndEscape(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 24))
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 34})
	m, _ = update(t, m, keyCtrlF())
	if m.sidebarFocused {
		t.Fatalf("ctrl+f focused sidebar when width is too narrow")
	}

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})
	m, _ = update(t, m, keyRunes("/sidebar off"))
	m, _ = update(t, m, keyEnter())
	m, _ = update(t, m, keyCtrlF())
	if m.sidebarFocused {
		t.Fatalf("ctrl+f focused hidden sidebar")
	}

	m, _ = update(t, m, keyRunes("/sidebar on"))
	m, _ = update(t, m, keyEnter())
	m, _ = update(t, m, keyCtrlF())
	if !m.sidebarFocused {
		t.Fatalf("ctrl+f should focus visible sidebar")
	}
	m, _ = update(t, m, keyEsc())
	if m.sidebarFocused {
		t.Fatalf("esc did not exit sidebar focus")
	}
}

func TestModelSidebarScrollDoesNotMoveInput(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 40))
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	before := lineContaining(stripANSI(m.View()), "ask anything, or /help for commands")
	if before < 0 {
		t.Fatalf("input line missing before sidebar scroll:\n%s", m.View())
	}

	x, y := sidebarPoint(t, m, 1)
	for i := 0; i < 6; i++ {
		m, _ = update(t, m, tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonWheelDown})
	}

	after := lineContaining(stripANSI(m.View()), "ask anything, or /help for commands")
	if after != before {
		t.Fatalf("input line moved after sidebar scroll: before=%d after=%d\n%s", before, after, m.View())
	}
}

func TestModelMouseWheelLeftSideStillScrollsTranscript(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 24))
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	m, _ = update(t, m, tea.MouseMsg{X: 0, Y: 0, Button: tea.MouseButtonWheelUp})
	if m.scrollOffset != 3 {
		t.Fatalf("left-side wheel should scroll transcript offset = %d, want 3", m.scrollOffset)
	}
	if m.sidebarScrollOffsets != [sidebarDocumentCount]int{} {
		t.Fatalf("left-side wheel should not scroll sidebar offsets: %#v", m.sidebarScrollOffsets)
	}
}

func TestSidebarBottomDoesNotReplaceContentWithEllipsis(t *testing.T) {
	doc := sidebarDocument{
		Title: "Plan",
		Content: strings.Join([]string{
			"# PLAN",
			"",
			"## Goal",
			"",
			"Collect project positioning details.",
			"",
			"## Strategy",
			"",
			numberedLines("strategy line", 12),
			"",
			"## Milestone",
			"",
			"final milestone line",
		}, "\n"),
	}
	offset := maxSidebarScrollOffset(doc, sidebarWidth, 12)
	box := stripANSI(renderSidebarBox(doc, sidebarWidth, 12, offset))

	if !strings.Contains(box, "final milestone line") {
		t.Fatalf("bottom sidebar view missing final content:\n%s", box)
	}
	if strings.Contains(box, "...") {
		t.Fatalf("bottom sidebar view should not show trailing ellipsis:\n%s", box)
	}
}

func TestSidebarBoxHidesMatchingRedundantDocumentHeading(t *testing.T) {
	tests := []struct {
		name    string
		title   string
		content string
		hidden  string
		wants   []string
	}{
		{
			name:  "memory uppercase",
			title: "Memory",
			content: strings.Join([]string{
				"# MEMORY",
				"",
				"## Goal",
				"Remember the repo conventions.",
			}, "\n"),
			hidden: "# MEMORY",
			wants:  []string{"MEMORY", "## Goal", "Remember the repo"},
		},
		{
			name:  "plan title case",
			title: "Plan",
			content: strings.Join([]string{
				"# Plan",
				"",
				"## Strategy",
				"- Keep markdown lists readable",
			}, "\n"),
			hidden: "# Plan",
			wants:  []string{"PLAN", "## Strategy", "• Keep markdown lists"},
		},
		{
			name:  "todo uppercase",
			title: "Todo",
			content: strings.Join([]string{
				"# TODO",
				"",
				"- [ ] Add focused sidebar tests",
				"- [x] Keep completed tasks readable",
			}, "\n"),
			hidden: "# TODO",
			wants:  []string{"TODO", "[ ] Add focused", "[✓] Keep completed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := sidebarDocument{Title: tt.title, Content: tt.content}
			box := stripANSI(renderSidebarBox(doc, sidebarWidth, 14, 0))
			lines := strings.Split(box, "\n")

			if lineContainsAll(lines, tt.hidden) {
				t.Fatalf("sidebar body should hide redundant heading %q:\n%s", tt.hidden, box)
			}
			for _, want := range tt.wants {
				if !lineContainsAll(lines, want) {
					t.Fatalf("sidebar box missing %q:\n%s", want, box)
				}
			}
		})
	}
}

func TestSidebarBoxKeepsNonMatchingDocumentHeading(t *testing.T) {
	doc := sidebarDocument{
		Title: "Plan",
		Content: strings.Join([]string{
			"# Memory",
			"",
			"Plan content starts here.",
		}, "\n"),
	}

	box := stripANSI(renderSidebarBox(doc, sidebarWidth, 12, 0))
	lines := strings.Split(box, "\n")
	if !lineContainsAll(lines, "# Memory") {
		t.Fatalf("sidebar should keep non-matching document heading:\n%s", box)
	}
}

func TestSidebarMarkdownListsKeepMarkersWithText(t *testing.T) {
	doc := sidebarDocument{
		Title: "Plan",
		Content: strings.Join([]string{
			"# PLAN",
			"",
			"1. 分析项目代码，定位 README.md 中 sidebar 渲染提前换行的问题，并补充测试。",
			"2. 修复 PLAN 和 TODO 的列表换行。",
			"3. 运行 go test ./...",
			"4. 验证边界线位置不偏移。",
		}, "\n"),
	}

	box := stripANSI(renderSidebarBox(doc, sidebarWidth, 14, 0))
	lines := strings.Split(box, "\n")
	for _, want := range []string{"1. 分析项目", "2. 修复", "3. 运行", "4. 验证"} {
		if !lineContainsAll(lines, want) {
			t.Fatalf("sidebar list item missing inline marker/text %q:\n%s", want, box)
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "1." || trimmed == "2." || trimmed == "3." || trimmed == "4." {
			t.Fatalf("sidebar rendered dangling ordered marker:\n%s", box)
		}
	}
}

func TestSidebarMarkdownTasksKeepCheckboxWithText(t *testing.T) {
	doc := sidebarDocument{
		Title: "Todo",
		Content: strings.Join([]string{
			"# TODO",
			"",
			"- [ ] 检查 README.md 后面的长待办是否正常换行并保持缩进",
			"- [x] 已完成 sidebar 宽度计算",
		}, "\n"),
	}

	box := stripANSI(renderSidebarBox(doc, sidebarWidth, 12, 0))
	lines := strings.Split(box, "\n")
	for _, want := range []string{"[ ] 检查", "[✓] 已完成"} {
		if !lineContainsAll(lines, want) {
			t.Fatalf("sidebar task item missing inline checkbox/text %q:\n%s", want, box)
		}
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[ ]" || trimmed == "[✓]" {
			t.Fatalf("sidebar rendered dangling checkbox:\n%s", box)
		}
		if lipgloss.Width(line) > sidebarWidth {
			t.Fatalf("sidebar task line width = %d, want <= %d: %q\n%s", lipgloss.Width(line), sidebarWidth, line, box)
		}
	}
}

func TestModelRunningTickRefreshesSidebarDocuments(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "old memory")
	writeTestFile(t, sessionDir, "PLAN.md", "old plan")
	writeTestFile(t, sessionDir, "TODO.md", "old todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	if got := sidebarContent(m.sidebarDocuments, "Plan"); got != "old plan" {
		t.Fatalf("initial plan = %q, want old plan", got)
	}

	writeTestFile(t, sessionDir, "PLAN.md", "new plan from disk")
	m.spinnerFrame = 3 // next tick increments to 4, triggering sidebar reload
	m, cmd := update(t, m, runningTickMsg{})
	if cmd == nil {
		t.Fatalf("running tick did not schedule another tick")
	}
	if got := sidebarContent(m.sidebarDocuments, "Plan"); got != "new plan from disk" {
		t.Fatalf("refreshed plan = %q, want new plan from disk", got)
	}
}

func TestModelRunningTickClampsShortenedSidebarDocument(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", numberedLines("plan line", 24))
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})

	m.sidebarScrollOffsets[1] = 99
	m.clampSidebarScrollOffsets()
	if m.sidebarScrollOffsets[1] == 0 {
		t.Fatalf("long plan should allow a positive sidebar offset")
	}

	writeTestFile(t, sessionDir, "PLAN.md", "short plan")
	m.spinnerFrame = 3 // next tick increments to 4, triggering sidebar reload
	m, _ = update(t, m, runningTickMsg{})
	if m.sidebarScrollOffsets[1] != 0 {
		t.Fatalf("shortened plan offset = %d, want clamped 0", m.sidebarScrollOffsets[1])
	}
	plainView := stripANSI(m.View())
	if !strings.Contains(plainView, "short plan") {
		t.Fatalf("shortened plan should render after clamping:\n%s", plainView)
	}
}

func TestContextUsageStyleThresholds(t *testing.T) {
	if contextUsageStyle(49).GetForeground() != contextLowStyle.GetForeground() {
		t.Fatalf("context usage under 50%% should use low style")
	}
	if contextUsageStyle(50).GetForeground() != contextMediumStyle.GetForeground() {
		t.Fatalf("context usage at 50%% should use medium style")
	}
	if contextUsageStyle(75).GetForeground() != contextHighStyle.GetForeground() {
		t.Fatalf("context usage at 75%% should use high style")
	}
	plain := stripANSI(renderContextUsage("76%"))
	if !strings.Contains(plain, "▓▓▓▓▓▓▓░░░ 76%") {
		t.Fatalf("context usage display = %q, want progress bar", plain)
	}
}

func TestModelViewDoesNotRenderPipeCursor(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyRunes("hello"))

	view := m.View()
	if strings.Contains(view, "hello|") {
		t.Fatalf("view rendered pipe cursor:\n%s", view)
	}
	if !strings.Contains(view, "hello"+renderCursor()) {
		t.Fatalf("view missing visual cursor after input:\n%s", view)
	}
	if !strings.Contains(view, "hello") {
		t.Fatalf("view missing typed input:\n%s", view)
	}
}

func TestModelViewHidesInputCursorWhenSidebarFocused(t *testing.T) {
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", "plan")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})
	m, _ = update(t, m, keyRunes("hello"))

	m, _ = update(t, m, keyCtrlF())
	view := m.View()
	if strings.Contains(view, "hello"+renderCursor()) {
		t.Fatalf("focused sidebar view rendered input cursor:\n%s", view)
	}
	if !strings.Contains(view, "hello") {
		t.Fatalf("focused sidebar view missing typed input:\n%s", view)
	}

	m, _ = update(t, m, keyEsc())
	view = m.View()
	if !strings.Contains(view, "hello"+renderCursor()) {
		t.Fatalf("input cursor did not return after sidebar focus closed:\n%s", view)
	}
}

func TestModelViewHidesInputCursorWhenTerminalBlurred(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyRunes("hello"))

	m, _ = update(t, m, tea.BlurMsg{})
	view := m.View()
	if strings.Contains(view, "hello"+renderCursor()) {
		t.Fatalf("blurred terminal view rendered input cursor:\n%s", view)
	}
	if !strings.Contains(view, "hello") {
		t.Fatalf("blurred terminal view missing typed input:\n%s", view)
	}

	m, _ = update(t, m, tea.FocusMsg{})
	view = m.View()
	if !strings.Contains(view, "hello"+renderCursor()) {
		t.Fatalf("input cursor did not return after terminal focus:\n%s", view)
	}
}

func TestModelViewShowsRunningNoticeAboveInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return current }
	m.running = true
	m.runStartedAt = current.Add(-(2*time.Hour + 3*time.Minute + 4*time.Second))

	view := m.View()
	if !strings.Contains(stripANSI(view), "working... 2h 3m 4s • esc to interrupt") {
		t.Fatalf("view missing running notice:\n%s", view)
	}
	if strings.Contains(stripANSI(view), "› ✦ working...") || strings.Contains(stripANSI(view), "› ✧ working...") {
		t.Fatalf("running notice rendered inside input:\n%s", view)
	}

	lines := strings.Split(stripANSI(view), "\n")
	noticeLine := lineContaining(stripANSI(view), "working...")
	inputLine := lineContaining(stripANSI(view), "message will be queued, or /cancel")
	if inputLine < 0 {
		t.Fatalf("view missing queue placeholder text:\n%s", view)
	}
	if noticeLine > inputLine {
		t.Fatalf("running notice should render above input:\n%s", view)
	}
	for _, line := range lines[noticeLine+1 : inputLine] {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("running notice should not have a blank line before input:\n%s", view)
		}
	}
	if noticeLine == 0 || strings.TrimSpace(lines[noticeLine-1]) != "" {
		t.Fatalf("running notice should have a blank line above it:\n%s", view)
	}
}

func TestModelRunningNoticeUsesBlinkingSparkle(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 5, 0, time.UTC)
	m.now = func() time.Time { return current }
	m.running = true
	m.runStartedAt = current.Add(-5 * time.Second)

	m.spinnerFrame = 0
	first := stripANSI(m.renderRunningNotice(72))
	m.spinnerFrame = 1
	second := stripANSI(m.renderRunningNotice(72))
	m.spinnerFrame = 2
	third := stripANSI(m.renderRunningNotice(72))
	m.spinnerFrame = 4
	fifth := stripANSI(m.renderRunningNotice(72))

	if !strings.Contains(first, "✦ working... 5s • esc to interrupt") {
		t.Fatalf("first frame missing filled sparkle working notice:\n%s", first)
	}
	if !strings.Contains(second, "✦ working... 5s • esc to interrupt") {
		t.Fatalf("second frame should keep filled sparkle for slower blink:\n%s", second)
	}
	if !strings.Contains(third, "✦ working... 5s • esc to interrupt") {
		t.Fatalf("third frame should keep filled sparkle for slower blink:\n%s", third)
	}
	if !strings.Contains(fifth, "✧ working... 5s • esc to interrupt") {
		t.Fatalf("fifth frame missing hollow sparkle working notice:\n%s", fifth)
	}
	if strings.Contains(first, "[ WORKING ]") || strings.Contains(first, "▰") || strings.Contains(first, "▱") || strings.Contains(first, "⬢") || strings.Contains(first, "⬡") {
		t.Fatalf("running notice still uses old working bar:\n%s", first)
	}
}

func TestRenderWorkingTextShimmers(t *testing.T) {
	first := renderWorkingText(11)
	second := renderWorkingText(12)
	if stripANSI(first) != "working..." || stripANSI(second) != "working..." {
		t.Fatalf("shimmer changed visible working text: first=%q second=%q", stripANSI(first), stripANSI(second))
	}
	firstBefore, firstShimmer, _ := workingShimmerSegments(workingNoticeText, workingGlimmerIndex(11, len([]rune(workingNoticeText))))
	secondBefore, secondShimmer, _ := workingShimmerSegments(workingNoticeText, workingGlimmerIndex(12, len([]rune(workingNoticeText))))
	if firstShimmer == "" || secondShimmer == "" {
		t.Fatalf("expected visible shimmer segments, got first=%q second=%q", firstShimmer, secondShimmer)
	}
	if firstShimmer == secondShimmer {
		t.Fatalf("shimmer did not move between frames: first=%q second=%q", firstShimmer, secondShimmer)
	}
	if len([]rune(secondBefore)) <= len([]rune(firstBefore)) {
		t.Fatalf("shimmer should move left-to-right: first before=%q second before=%q", firstBefore, secondBefore)
	}
	if workingTextStyle.GetForeground() != lipgloss.Color(claudeWorkingHex) {
		t.Fatalf("working text foreground = %q, want Claude working color %q", workingTextStyle.GetForeground(), claudeWorkingHex)
	}
	if workingShimmerStyle.GetForeground() != lipgloss.Color(claudeShimmerHex) {
		t.Fatalf("working shimmer foreground = %q, want Claude shimmer color %q", workingShimmerStyle.GetForeground(), claudeShimmerHex)
	}
}

func TestFormatDurationDoesNotPadSingleDigitUnits(t *testing.T) {
	got := formatDuration(time.Minute + 4*time.Second)
	if got != "1m 4s" {
		t.Fatalf("formatDuration(1m4s) = %q, want 1m 4s", got)
	}
}

func TestModelRunningTickAdvancesSpinner(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return current }
	m.running = true
	m.runStartedAt = current

	before := m.workingFrame()
	m, cmd := update(t, m, runningTickMsg{})
	if cmd == nil {
		t.Fatalf("running tick did not schedule another tick")
	}
	after := m.workingFrame()
	if before != after {
		t.Fatalf("sparkle blink advanced too quickly after one tick: before=%q after=%q", before, after)
	}
	for i := 0; i < 3; i++ {
		m, cmd = update(t, m, runningTickMsg{})
		if cmd == nil {
			t.Fatalf("running tick %d did not schedule another tick", i+2)
		}
		after = m.workingFrame()
	}
	if before == after {
		t.Fatalf("sparkle blink did not advance after four ticks: before=%q after=%q", before, after)
	}
}

func TestRunningTickInterval(t *testing.T) {
	if runningTickEvery != 150*time.Millisecond {
		t.Fatalf("runningTickEvery = %s, want 150ms", runningTickEvery)
	}
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		sessionID:    "sess-1",
		sessionDir:   "/tmp/sess-1",
		workDir:      "/tmp/work",
		model:        "fake-model",
		contextUsage: "7%",
		truncatedSeq: -1,
	}
}

func TestPermissionsCommandUpdatesModeAndPersistsSettings(t *testing.T) {
	runner := newFakeRunner()
	home := t.TempDir()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	next, _ := m.handleSlashCommand("/permissions approve")
	m = next.(Model)
	if got := runner.PermissionSnapshot().EffectiveMode; got != permission.ModeApprove {
		t.Fatalf("EffectiveMode = %q, want approve", got)
	}
	loaded, err := settings.Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.TUI.Permissions.Mode; got != string(permission.ModeApprove) {
		t.Fatalf("persisted mode = %q, want approve", got)
	}
}

func TestPermissionsFullAccessCommandOpensWarningAndConfirmActivates(t *testing.T) {
	runner := newFakeRunner()
	home := t.TempDir()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	next, _ := m.handleSlashCommand("/permissions full-access")
	m = next.(Model)
	if m.permissionForm == nil {
		t.Fatal("/permissions full-access did not open warning form")
	}
	if m.permissionForm.stage != permissionFormStageFullAccessWarning {
		t.Fatalf("stage = %v, want full access warning", m.permissionForm.stage)
	}
	snap := runner.PermissionSnapshot()
	if snap.EffectiveMode == permission.ModeFullAccess {
		t.Fatal("Full Access activated before confirmation")
	}

	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("full access confirmation command is nil")
	}
	m, _ = update(t, m, cmd())
	snap = runner.PermissionSnapshot()
	if snap.SelectedMode != permission.ModeFullAccess || snap.EffectiveMode != permission.ModeFullAccess {
		t.Fatalf("snapshot = %+v, want selected/effective full access", snap)
	}
	loaded, err := settings.Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.TUI.Permissions.Mode; got != string(permission.ModeFullAccess) {
		t.Fatalf("persisted mode = %q, want full access", got)
	}
	if loaded.TUI.Permissions.FullAccessWarningRemembered {
		t.Fatal("FullAccessWarningRemembered = true, want false")
	}
}

func TestPermissionsSelectorApproveMode(t *testing.T) {
	runner := newFakeRunner()
	home := t.TempDir()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	m, _ = update(t, m, keyRunes("/permissions"))
	m, _ = update(t, m, keyEnter())
	if m.permissionForm == nil {
		t.Fatal("/permissions did not open form")
	}
	m, _ = update(t, m, keyDown())
	m, cmd := update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("permissions selection command is nil")
	}
	m, _ = update(t, m, cmd())

	if m.permissionForm != nil {
		t.Fatal("permission form still open")
	}
	if got := runner.PermissionSnapshot().EffectiveMode; got != permission.ModeApprove {
		t.Fatalf("EffectiveMode = %q, want approve", got)
	}
	loaded, err := settings.Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.TUI.Permissions.Mode; got != string(permission.ModeApprove) {
		t.Fatalf("persisted mode = %q, want approve", got)
	}
}

func TestPermissionsSelectorFullAccessShowsWarningBeforeActivation(t *testing.T) {
	runner := newFakeRunner()
	home := t.TempDir()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	m, _ = update(t, m, keyRunes("/permissions"))
	m, _ = update(t, m, keyEnter())
	m, _ = update(t, m, keyDown())
	m, _ = update(t, m, keyDown())
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatal("selecting Full Access should open warning without completing")
	}
	if m.permissionForm == nil || m.permissionForm.stage != permissionFormStageFullAccessWarning {
		t.Fatalf("permission form = %#v, want warning stage", m.permissionForm)
	}
	if got := runner.PermissionSnapshot().EffectiveMode; got == permission.ModeFullAccess {
		t.Fatal("Full Access activated before warning confirmation")
	}

	m, cmd = update(t, m, keyEnter())
	if cmd == nil {
		t.Fatal("warning confirmation command is nil")
	}
	m, _ = update(t, m, cmd())
	if got := runner.PermissionSnapshot().EffectiveMode; got != permission.ModeFullAccess {
		t.Fatalf("EffectiveMode = %q, want full access", got)
	}
}

func TestPermissionsFullAccessRememberActivatesAndPersistsAcknowledgement(t *testing.T) {
	runner := newFakeRunner()
	home := t.TempDir()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	next, _ := m.handleSlashCommand("/permissions full-access remember")
	m = next.(Model)
	snap := runner.PermissionSnapshot()
	if snap.EffectiveMode != permission.ModeFullAccess {
		t.Fatalf("EffectiveMode = %q, want full access", snap.EffectiveMode)
	}
	if !snap.FullAccessRemembered {
		t.Fatal("FullAccessRemembered = false, want true")
	}
	loaded, err := settings.Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !loaded.TUI.Permissions.FullAccessWarningRemembered {
		t.Fatal("persisted FullAccessWarningRemembered = false, want true")
	}
}

func TestPermissionsFullAccessConfirmActivatesWithoutRemembering(t *testing.T) {
	runner := newFakeRunner()
	home := t.TempDir()
	m := NewModel(context.Background(), runner, Config{HomeDir: home})

	next, _ := m.handleSlashCommand("/permissions full-access confirm")
	m = next.(Model)
	snap := runner.PermissionSnapshot()
	if snap.EffectiveMode != permission.ModeFullAccess {
		t.Fatalf("EffectiveMode = %q, want full access", snap.EffectiveMode)
	}
	if snap.FullAccessRemembered {
		t.Fatal("FullAccessRemembered = true, want false")
	}
	loaded, err := settings.Load(home)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.TUI.Permissions.FullAccessWarningRemembered {
		t.Fatal("persisted FullAccessWarningRemembered = true, want false")
	}
}

func TestPermissionsStatuslineIsOptionalAndRenderable(t *testing.T) {
	if containsString(defaultStatuslineItems, "permissions") {
		t.Fatal("permissions must not be in default statusline items")
	}
	runner := newFakeRunner()
	runner.SetPermissionMode(permission.ModeApprove, false)
	m := NewModel(context.Background(), runner, Config{})
	m.statuslineItems = []string{"permissions"}
	if got := m.renderStatuslineItem("permissions"); !strings.Contains(got, "Approve for me") {
		t.Fatalf("permissions statusline = %q, want mode label", got)
	}
}

func TestFullAccessWarningRendersAtBottom(t *testing.T) {
	runner := newFakeRunner()
	runner.ActivateFullAccess(false)
	m := NewModel(context.Background(), runner, Config{})
	got := m.renderKeybinds(80)
	if !strings.Contains(got, "[ full access ]") {
		t.Fatalf("keybinds = %q, want full access warning", got)
	}
}

func TestUnrememberedFullAccessStartupShowsWarningEntry(t *testing.T) {
	runner := newFakeRunner()
	runner.permissionState = permission.NewState(permission.ModeFullAccess, false)
	m := NewModel(context.Background(), runner, Config{})
	if !entriesContain(m.entries, "system", "Full Access is selected but not remembered") {
		t.Fatalf("entries = %#v, want startup warning", m.entries)
	}
}

func TestPermissionStateChangedRefreshesSnapshot(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	if got := m.permissionSnapshot.SessionGrantCount; got != 0 {
		t.Fatalf("initial grant count = %d, want 0", got)
	}
	runner.permissionState.AddGrant(permission.GrantForRequest(permission.Request{
		ToolName:  "bash",
		CWD:       runner.workDir,
		Workspace: runner.workDir,
		Source:    permission.SourceMain,
	}))

	next, _ := update(t, m, permissionStateChangedMsg{})
	m = next
	if got := m.permissionSnapshot.SessionGrantCount; got != 1 {
		t.Fatalf("refreshed grant count = %d, want 1", got)
	}
}

func testQueuedPrompts(texts ...string) []queuedPrompt {
	prompts := make([]queuedPrompt, 0, len(texts))
	for _, text := range texts {
		prompts = append(prompts, queuedPrompt{text: text, mode: collaboration.ModeDefault})
	}
	return prompts
}

func queuedPromptTexts(prompts []queuedPrompt) []string {
	texts := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		texts = append(texts, prompt.text)
	}
	return texts
}

func historyRecord(seq int64, runID string, msg schema.Message) session.MessageRecord {
	return session.MessageRecord{
		Seq:     seq,
		RunID:   runID,
		Time:    time.Date(2026, 5, 17, 12, 0, int(seq), 0, time.Local),
		Kind:    session.MessageKindNormal,
		Message: msg,
	}
}

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	typed, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	return typed, cmd
}

func selectableTranscriptModel(t *testing.T, body string) (Model, *string) {
	t.Helper()
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.entries = nil
	m.appendEntry("assistant", "", body, false)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	copied := ""
	m.copySelection = func(text string) error {
		copied = text
		return nil
	}
	return m, &copied
}

func selectableSidebarModel(t *testing.T, plan string) (Model, *string) {
	t.Helper()
	workDir := t.TempDir()
	sessionDir := t.TempDir()
	writeTestFile(t, workDir, "MEMORY.md", "memory")
	writeTestFile(t, sessionDir, "PLAN.md", plan)
	writeTestFile(t, sessionDir, "TODO.md", "todo")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 34})
	copied := ""
	m.copySelection = func(text string) error {
		copied = text
		return nil
	}
	return m, &copied
}

func findTranscriptText(t *testing.T, m Model, text string) (int, int) {
	t.Helper()
	layout := m.transcriptLayout(m.chatWidth(), m.transcriptHeight())
	for line := layout.visibleStart; line < layout.visibleEnd; line++ {
		col := strings.Index(layout.plainLines[line], text)
		if col >= 0 {
			return line - layout.visibleStart, col
		}
	}
	t.Fatalf("did not find %q in visible transcript lines: %#v", text, layout.plainLines[layout.visibleStart:layout.visibleEnd])
	return 0, 0
}

func findSidebarText(t *testing.T, m Model, text string) (int, int) {
	t.Helper()
	layout := m.sidebarLayout(m.sidebarWidth(), m.transcriptHeight())
	for line, plain := range layout.plainLines {
		col := strings.Index(plain, text)
		if col >= 0 {
			return line, col
		}
	}
	t.Fatalf("did not find %q in visible sidebar lines: %#v", text, layout.plainLines)
	return 0, 0
}

func sidebarSelectionPoint(t *testing.T, m Model, line int, col int) (int, int) {
	t.Helper()
	contentWidth, _ := m.contentDimensions()
	return viewPaddingLeft + contentWidth + sidebarGap + sidebarDividerWidth + col, viewPaddingTop + line
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keyPaste(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true}
}

func keyEnter() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}

func keyShiftEnter() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlJ}
}

func keyEsc() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEsc}
}

func keyCtrlC() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlC}
}

func keyCtrlF() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlF}
}

func keyCtrlO() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlO}
}

func keyTab() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyTab}
}

func keyShiftTab() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyShiftTab}
}

func keyUp() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyUp}
}

func keyDown() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyDown}
}

func keyLeft() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyLeft}
}

func keyRight() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRight}
}

func keyBackspace() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyBackspace}
}

func keyDelete() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyDelete}
}

func keyPgUp() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyPgUp}
}

func keyPgDown() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyPgDown}
}

func keyHome() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyHome}
}

func keyEnd() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnd}
}

func keySpace() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeySpace}
}

func renderedInputCursorPosition(m Model) (int, int, bool) {
	lines := strings.Split(stripANSI(m.renderInput(m.innerWidth())), "\n")
	for row, line := range lines {
		if col := strings.IndexRune(line, '▌'); col >= 0 {
			return row, lipgloss.Width(line[:col]), true
		}
	}
	m.clampInputCursor()
	points := m.inputCursorPoints()
	if m.inputCursor >= len(points) {
		return 0, 0, false
	}
	point := points[m.inputCursor]
	prefixWidth := lipgloss.Width("> ")
	if point.row > 0 {
		prefixWidth = lipgloss.Width("  ")
	}
	return point.row + 1, prefixWidth + point.col, true
}

func renderedInputContentRows(m Model) []string {
	lines := strings.Split(stripANSI(m.renderInput(m.innerWidth())), "\n")
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " ")
		if trimmed == "" {
			rows = append(rows, "")
			continue
		}
		trimmed = strings.ReplaceAll(trimmed, "▌", "")
		if strings.Trim(trimmed, "─") == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "> ") {
			rows = append(rows, strings.TrimPrefix(trimmed, "> "))
			continue
		}
		if strings.HasPrefix(trimmed, "! ") {
			rows = append(rows, strings.TrimPrefix(trimmed, "! "))
			continue
		}
		if strings.HasPrefix(trimmed, "  ") {
			rows = append(rows, strings.TrimPrefix(trimmed, "  "))
		}
	}
	return rows
}

func inputTextOrigin(m Model) (int, int) {
	x := viewPaddingLeft + inputStyle.GetHorizontalFrameSize()/2 + lipgloss.Width("> ")
	return m.inputContentY(), x
}

func longPastedInputSample() string {
	return strings.Join([]string{
		"Intro paragraph with enough words to soft wrap in a narrow terminal viewport.",
		"",
		"  indented command --flag=value --json '{\"alpha\":1,\"beta\":[\"one\",\"two\",\"three\"]}'",
		"中文内容应该按行保留，并且不会因为 viewport 被截断而乱序。",
		"- bullet one keeps its marker",
		"- bullet two includes a longer sentence that wraps across the available input width",
		`{"nested":{"path":["one","two","three"],"enabled":true},"message":"long json should stay ordered"}`,
		"final line",
	}, "\n")
}

func reportedLongPastedInputSample() string {
	return strings.Join([]string{
		"• 你说得对。真正漏掉的是这个路径：WindowSizeMsg 里把高度强制抬到了 minHeight=20，所以真实终端如果只有 14/16/18 行，TUI 仍然渲染 20 行，终端自然只显示底部，和原问题一致。",
		"",
		"  我已经修了：",
		"",
		"  - internal/tui/model.go:242：窗口高度不再 clamp 到 20，改为使用真实高度。",
		"  - internal/tui/view.go:170：布局按真实高度算。",
		"  - internal/tui/view.go:917：短窗口下 transcript 最小保留高度会动态下降，避免和输入框一起撑爆总高度。",
		"  - internal/tui/model_test.go:1661：新增复现测试，真实窗口 14 行时长输入渲染不能超过 14 行。这个测试在修复前失败：view height = 20, want <= actual window height 14。",
		"",
		"  也保留了之前的：",
		"",
		"  - 长输入 viewport 跟随光标",
		"  - 光标常亮",
		"  - 行宽不超过窗口",
		"  - 混合内容不乱序",
		"",
		"  验证已跑：",
		"",
		"  go test ./internal/tui -count=1",
		"  go test ./...",
		"",
		"  都通过。",
	}, "\n")
}

func assertViewFitsTerminal(t *testing.T, m Model) {
	t.Helper()
	view := stripANSI(m.View())
	if got := lipgloss.Height(view); got > m.height {
		t.Fatalf("view height = %d, want <= window height %d", got, m.height)
	}
	for i, line := range strings.Split(view, "\n") {
		maxWidth := max(m.width-1, 1)
		if got := lipgloss.Width(line); got > maxWidth {
			t.Fatalf("view line %d width = %d, want <= safe window width %d\n%q", i, got, maxWidth, line)
		}
	}
}

func assertQuitCommand(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("quit command is nil")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("command returned %T, want tea.QuitMsg", msg)
	}
}

func entriesContain(entries []entry, role string, text string) bool {
	for _, entry := range entries {
		if entry.role == role && strings.Contains(entry.body, text) {
			return true
		}
	}
	return false
}

func countEntriesContaining(entries []entry, role string, text string) int {
	count := 0
	for _, entry := range entries {
		if entry.role == role && strings.Contains(entry.body, text) {
			count++
		}
	}
	return count
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

func assertSlashDropdownLineCount(t *testing.T, rendered string) {
	t.Helper()
	plain := stripANSI(rendered)
	lines := nonEmptyLines(plain)
	if len(lines) != len(slashCommands) {
		t.Fatalf("slash dropdown wrapped selected row: got %d lines, want %d\n%s", len(lines), len(slashCommands), rendered)
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "paths" || trimmed == "session" {
			t.Fatalf("slash dropdown rendered a dangling wrapped word %q:\n%s", trimmed, rendered)
		}
	}
}

func nonEmptyLines(s string) []string {
	rawLines := strings.Split(s, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func lineContainsAll(lines []string, fragments ...string) bool {
	for _, line := range lines {
		matches := true
		for _, fragment := range fragments {
			if !strings.Contains(line, fragment) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func writeTestFile(t *testing.T, root string, rel string, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func settingsJSONPath(home string) string {
	return filepath.Join(home, ".foxharness", "settings.json")
}

func readTUISettingsMap(t *testing.T, home string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(settingsJSONPath(home))
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	tui, ok := parsed["tui"].(map[string]any)
	if !ok {
		t.Fatalf("settings missing tui object: %#v", parsed)
	}
	return tui
}

func numberedLines(prefix string, count int) string {
	var b strings.Builder
	for i := 1; i <= count; i++ {
		if i > 1 {
			b.WriteByte('\n')
		}
		b.WriteString(prefix)
		b.WriteByte(' ')
		if i < 10 {
			b.WriteByte('0')
		}
		b.WriteString(strconv.Itoa(i))
	}
	return b.String()
}

func sidebarPoint(t *testing.T, m Model, index int) (int, int) {
	t.Helper()
	contentWidth, contentHeight := m.contentDimensions()
	heights := sidebarBoxHeights(sidebarDocumentAreaHeight(contentHeight, len(m.sidebarDocuments)), len(m.sidebarDocuments))
	if index < 0 || index >= len(heights) {
		t.Fatalf("sidebar index %d out of range for heights %#v", index, heights)
	}
	y := 0
	for i := 0; i < index; i++ {
		y += heights[i]
		y += sidebarSeparatorHeight
	}
	return viewPaddingLeft + contentWidth + sidebarGap, viewPaddingTop + y
}

func lineContaining(text string, fragment string) int {
	for i, line := range strings.Split(text, "\n") {
		if strings.Contains(line, fragment) {
			return i
		}
	}
	return -1
}

func fileMentionPaths(mentions []fileMention) []string {
	paths := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		paths = append(paths, mention.Path)
	}
	return paths
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sidebarContent(docs []sidebarDocument, title string) string {
	for _, doc := range docs {
		if doc.Title == title {
			return doc.Content
		}
	}
	return ""
}
