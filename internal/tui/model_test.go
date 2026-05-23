package tui

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tui/selector"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fakeRunner struct {
	sessionID  string
	sessionDir string
	workDir    string
	model      string

	runs         []string
	runErr       error
	runErrs      []error
	setModelErr  error
	newErr       error
	nextRunID    int
	planMode     bool
	contextUsage string
	history      []session.MessageRecord
	historyErr   error
	truncatedSeq int64
	checkpointer checkpoint.Checkpointer
}

func (r *fakeRunner) Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	r.runs = append(r.runs, prompt)
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

func (r *fakeRunner) Checkpointer() checkpoint.Checkpointer {
	return r.checkpointer
}

func (r *fakeRunner) PlanMode() bool {
	return r.planMode
}

func (r *fakeRunner) SetPlanMode(enabled bool) {
	r.planMode = enabled
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
		"◆ Bash (date)",
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
		{name: "read", body: formatToolInvocation("read_file", `{"path":"internal/foo.go"}`), want: "◆ Read (internal/foo.go)"},
		{name: "write", body: formatToolInvocation("write_file", `{"path":"cmd/app.go"}`), want: "◆ Write (cmd/app.go)"},
		{name: "edit", body: formatToolInvocation("edit_file", `{"path":"internal/app.go"}`), want: "◆ Edit (internal/app.go)"},
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

func TestAssistantMessagesRenderMarkdown(t *testing.T) {
	rendered := renderAssistantEntry(entry{
		role:  "assistant",
		title: "foxharness",
		body:  "Today is **Sunday, May 17, 2026**.\n\n- current day\n- terminal markdown",
		time:  time.Date(2026, 5, 17, 15, 38, 44, 0, time.Local),
	}, 100)
	plainRendered := stripANSI(rendered)

	for _, forbidden := range []string{"**Sunday", "**.", "- current day"} {
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

func TestNewSessionClearsRestoredHistory(t *testing.T) {
	runner := newFakeRunner()
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
	if len(m.inputHistory) != 0 {
		t.Fatalf("/new inputHistory = %#v, want empty", m.inputHistory)
	}
	if !entriesContain(m.entries, "command", "ID       sess-new") {
		t.Fatalf("/new did not render new session details: %#v", m.entries)
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
	if !entriesContain(m.entries, "command", "/session") ||
		!entriesContain(m.entries, "command", "show current session paths") {
		t.Fatalf("/help did not render commands: %#v", m.entries)
	}

	m, _ = update(t, m, keyRunes("/session"))
	m, _ = update(t, m, keyEnter())
	if !entriesContain(m.entries, "command", "ID       sess-1") ||
		!entriesContain(m.entries, "command", "Dir      /tmp/sess-1") {
		t.Fatalf("/session did not render session details: %#v", m.entries)
	}

	m, _ = update(t, m, keyRunes("/clear"))
	m, _ = update(t, m, keyEnter())
	if len(m.entries) != 0 {
		t.Fatalf("/clear entries len = %d, want 0", len(m.entries))
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
	for _, want := range []string{"❯", "/session", "/clear", "/new", "/model", "/cancel", "/sidebar", "/help", "/exit"} {
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
	if command, ok := m.selectedSlashCommand(); !ok || command.Name != "/clear" {
		t.Fatalf("selected slash command after down = %#v, %v; want /clear", command, ok)
	}
	m, _ = update(t, m, keyUp())
	if command, ok := m.selectedSlashCommand(); !ok || command.Name != "/session" {
		t.Fatalf("selected slash command after up = %#v, %v; want /session", command, ok)
	}

	m, _ = update(t, m, keyTab())
	if got := string(m.input); got != "/session" {
		t.Fatalf("input after tab = %q, want /session", got)
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
	if m.scrollOffset != 1 {
		t.Fatalf("scrollOffset after wheel up = %d, want 1", m.scrollOffset)
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

func TestFragmentedSGRMousePayloadDoesNotEnterInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.input = []rune("draft")

	m, _ = update(t, m, keyRunes("[<64;43;17M"))

	if got := string(m.input); got != "draft" {
		t.Fatalf("input after fragmented mouse payload = %q, want draft", got)
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
	if m.planMode {
		t.Fatalf("initial planMode = true, want false")
	}
	if !strings.Contains(m.View(), "plan mode off") || !strings.Contains(m.View(), "shift + tab to cycle") {
		t.Fatalf("plan mode off hint missing:\n%s", m.View())
	}

	m, _ = update(t, m, keyShiftTab())
	if !m.planMode || !runner.planMode {
		t.Fatalf("plan mode was not enabled: model=%v runner=%v", m.planMode, runner.planMode)
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
	if m.planMode || runner.planMode {
		t.Fatalf("plan mode was not disabled: model=%v runner=%v", m.planMode, runner.planMode)
	}
	if m.status != "Plan mode disabled" {
		t.Fatalf("status = %q, want Plan mode disabled", m.status)
	}
	if !strings.Contains(m.View(), "plan mode off") || !strings.Contains(m.View(), "shift + tab to cycle") {
		t.Fatalf("plan mode off hint missing after disabling:\n%s", m.View())
	}
}

func TestModelTogglesPlanModeWhileRunningForNextRun(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m.running = true

	m, _ = update(t, m, keyShiftTab())
	if !m.planMode || !runner.planMode {
		t.Fatalf("plan mode was not enabled while running: model=%v runner=%v", m.planMode, runner.planMode)
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

func TestModelInputHistoryRestoresDraft(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("saved task"))
	m, cmd := update(t, m, keyEnter())
	m, _ = update(t, m, cmd())

	m, _ = update(t, m, keyRunes("draft"))
	m, _ = update(t, m, keyUp())
	if got := string(m.input); got != "saved task" {
		t.Fatalf("history recall = %q, want saved task", got)
	}

	m, _ = update(t, m, keyDown())
	if got := string(m.input); got != "draft" {
		t.Fatalf("restored draft = %q, want draft", got)
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
	if len(m.queuedPrompts) != 1 || m.queuedPrompts[0] != "queued follow-up" {
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

func TestModelRunningNoticeShowsQueuedPromptPreviews(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 5, 0, time.UTC)
	m.now = func() time.Time { return current }
	m.running = true
	m.runStartedAt = current.Add(-5 * time.Second)
	m.queuedPrompts = []string{
		"second task",
		"third task\nwith newline",
		strings.Repeat("long ", 40),
		"fourth task",
	}

	notice := stripANSI(m.renderRunningNotice(72))
	for _, want := range []string{
		"[ WORKING ]",
		"elapsed 05s",
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
	if got := strings.Join(m.queuedPrompts, ","); got != "second task,third task" {
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
	if got := strings.Join(m.queuedPrompts, ","); got != "third task" {
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
	if got := strings.Join(m.queuedPrompts, ","); got != "second task,/model next-model,third task" {
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
	for _, want := range []string{"SYSTEM session", "Interactive session started. Type /help for commands.", "FOXHARNESS", "fake-model", "git -", "Context 7%", "sid sess-1", "> ▌ ask anything, or /help for commands"} {
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
	writeTestFile(t, workDir, "MEMORY.md", "# Memory\n\nRemember the repo conventions.")
	writeTestFile(t, workDir, "PLAN.md", "stale project plan")
	writeTestFile(t, workDir, "TODO.md", "stale project todo")
	writeTestFile(t, sessionDir, "PLAN.md", "- Build right sidebar")
	writeTestFile(t, sessionDir, "TODO.md", "- [ ] Add tests")

	runner := newFakeRunner()
	runner.workDir = workDir
	runner.sessionDir = sessionDir
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
	if m.sidebarScrollOffsets[1] != 1 {
		t.Fatalf("plan sidebar offset after wheel down = %d, want 1", m.sidebarScrollOffsets[1])
	}
	if m.sidebarScrollOffsets[0] != 0 || m.sidebarScrollOffsets[2] != 0 {
		t.Fatalf("scrolling plan should not affect other sidebar offsets: %#v", m.sidebarScrollOffsets)
	}

	plainView := stripANSI(m.View())
	if strings.Contains(plainView, "plan line 01") {
		t.Fatalf("scrolled sidebar plan should hide the first line:\n%s", plainView)
	}
	if !strings.Contains(plainView, "03 plan") {
		t.Fatalf("scrolled sidebar plan should show later content:\n%s", plainView)
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
	if !strings.Contains(plainView, "03 plan") {
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
	if m.scrollOffset != 1 {
		t.Fatalf("left-side wheel should scroll transcript offset = %d, want 1", m.scrollOffset)
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

func TestModelViewShowsRunningNoticeAboveInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	current := time.Date(2026, 5, 17, 12, 0, 58, 0, time.UTC)
	m.now = func() time.Time { return current }
	m.running = true
	m.runStartedAt = current.Add(-58 * time.Second)

	view := m.View()
	if !strings.Contains(view, "[ WORKING ]") || !strings.Contains(view, "elapsed 58s") || !strings.Contains(view, "esc to interrupt") {
		t.Fatalf("view missing running notice:\n%s", view)
	}
	if strings.Contains(view, "› [ WORKING ]") {
		t.Fatalf("running notice rendered inside input:\n%s", view)
	}

	noticeIndex := strings.Index(view, "[ WORKING ]")
	inputIndex := strings.Index(view, "message will be queued, or /cancel")
	if inputIndex < 0 {
		t.Fatalf("view missing queue placeholder text:\n%s", view)
	}
	if noticeIndex > inputIndex {
		t.Fatalf("running notice should render above input:\n%s", view)
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
	if before == after {
		t.Fatalf("spinner frame did not advance: before=%q after=%q", before, after)
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

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
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
