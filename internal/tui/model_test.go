package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Zts0hg/foxharness/internal/engine"
	tea "github.com/charmbracelet/bubbletea"
)

type fakeRunner struct {
	sessionID  string
	sessionDir string
	workDir    string
	model      string

	runs      []string
	runErr    error
	newErr    error
	nextRunID int
}

func (r *fakeRunner) Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error) {
	r.runs = append(r.runs, prompt)
	r.nextRunID++
	runID := "run-1"
	if r.nextRunID > 1 {
		runID = "run-2"
	}
	reporter.OnRunStart(ctx, r.sessionID, runID)
	reporter.OnToolCall(ctx, "read_file", `{"path":"go.mod"}`)
	reporter.OnToolResult(ctx, "read_file", "module github.com/Zts0hg/foxharness", false)
	if r.runErr != nil {
		reporter.OnRunError(ctx, r.sessionID, runID, r.runErr)
		return nil, r.runErr
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
	if !entriesContain(m.entries, "tool", "module github.com/Zts0hg/foxharness") {
		t.Fatalf("entries missing tool result: %#v", m.entries)
	}
	if entriesContain(m.entries, "system", "Metrics:") || entriesContain(m.entries, "system", "Trace:") {
		t.Fatalf("run completion details should stay out of transcript entries: %#v", m.entries)
	}
	if !strings.Contains(m.status, "run-1") {
		t.Fatalf("status = %q, want completed run id", m.status)
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

func TestModelSlashCommands(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/help"))
	m, cmd := update(t, m, keyEnter())
	if cmd != nil {
		t.Fatalf("/help returned unexpected command")
	}
	if !entriesContain(m.entries, "system", "/session") {
		t.Fatalf("/help did not render commands: %#v", m.entries)
	}

	m, _ = update(t, m, keyRunes("/session"))
	m, _ = update(t, m, keyEnter())
	if !entriesContain(m.entries, "system", "Session: sess-1") {
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
	if !entriesContain(m.entries, "system", "Switched to session sess-new") {
		t.Fatalf("/new did not render switch message: %#v", m.entries)
	}
}

func TestModelBlocksInputWhileRunIsActiveAndCancels(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("long task"))
	m, _ = update(t, m, keyEnter())
	if !m.running {
		t.Fatalf("model running = false, want true")
	}

	m, _ = update(t, m, keyRunes("ignored"))
	if got := string(m.input); got != "" {
		t.Fatalf("input while running = %q, want empty", got)
	}

	m, _ = update(t, m, keyEsc())
	if !strings.Contains(m.status, "Cancel") {
		t.Fatalf("status = %q, want cancel status", m.status)
	}
	if !entriesContain(m.entries, "system", "cancellation requested") {
		t.Fatalf("entries missing cancel notice: %#v", m.entries)
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
}

func TestModelViewContainsSessionAndInput(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{Model: "fake-model"})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	view := m.View()
	for _, want := range []string{"FOXHARNESS", "fake-model", "sess-1", "Message foxharness"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
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
	m.running = true

	view := m.View()
	if !strings.Contains(view, "Agent is running. Press Esc to request cancellation.") {
		t.Fatalf("view missing running notice:\n%s", view)
	}
	if strings.Contains(view, "> Agent is running. Press Esc to request cancellation.") {
		t.Fatalf("running notice rendered inside input:\n%s", view)
	}

	noticeIndex := strings.Index(view, "Agent is running. Press Esc to request cancellation.")
	inputIndex := strings.Index(view, "> Input locked until the current run completes.")
	if inputIndex < 0 {
		t.Fatalf("view missing locked input text:\n%s", view)
	}
	if noticeIndex > inputIndex {
		t.Fatalf("running notice should render above input:\n%s", view)
	}
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		sessionID:  "sess-1",
		sessionDir: "/tmp/sess-1",
		workDir:    "/tmp/work",
		model:      "fake-model",
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

func keyEsc() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEsc}
}

func keySpace() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeySpace}
}

func entriesContain(entries []entry, role string, text string) bool {
	for _, entry := range entries {
		if entry.role == role && strings.Contains(entry.body, text) {
			return true
		}
	}
	return false
}
