package tui

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
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
	newErr       error
	nextRunID    int
	planMode     bool
	contextUsage string
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

func (r *fakeRunner) ContextUsage() string {
	if r.contextUsage == "" {
		return "7%"
	}
	return r.contextUsage
}

func (r *fakeRunner) PlanMode() bool {
	return r.planMode
}

func (r *fakeRunner) SetPlanMode(enabled bool) {
	r.planMode = enabled
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
		"USER you",
		"SYSTEM run started",
		"TOOL call bash",
		"TOOL result bash",
		"SYSTEM thinking",
		"Planning turn",
		"Session: sess-1",
		"Run: run-1",
	} {
		if strings.Contains(plainView, forbidden) {
			t.Fatalf("view contains verbose fragment %q:\n%s", forbidden, view)
		}
	}
	for _, want := range []string{
		"You ",
		"hello, what's the day today?",
		"• Ran date",
		"└ 2026年 5月17日",
		"Foxharness",
		"answer: hello, what's the day today?",
	} {
		if !strings.Contains(plainView, want) {
			t.Fatalf("view missing compact fragment %q:\n%s", want, view)
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

	for _, forbidden := range []string{"**Sunday", "**.", "- current day"} {
		if strings.Contains(plainRendered, forbidden) {
			t.Fatalf("rendered assistant markdown contains raw markdown %q:\n%s", forbidden, rendered)
		}
	}
	for _, want := range []string{"Foxharness", "Sunday, May 17, 2026", "current day", "terminal markdown"} {
		if !strings.Contains(plainRendered, want) {
			t.Fatalf("rendered assistant markdown missing %q:\n%s", want, rendered)
		}
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("rendered assistant markdown missing terminal styling escape codes:\n%s", rendered)
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
	if !entriesContain(m.entries, "system", "### Session") ||
		!entriesContain(m.entries, "system", "- ID: `sess-1`") ||
		!entriesContain(m.entries, "system", "- Dir: `/tmp/sess-1`") {
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

func TestModelSlashSuggestionsAndTabCompletion(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m.inputHistory = []string{"previous task"}
	m, _ = update(t, m, keyRunes("/"))
	view := m.View()
	plainView := stripANSI(view)
	for _, want := range []string{"❯", "/session", "/clear", "/new", "/cancel", "/help", "/exit"} {
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

func TestModelSlashDropdownEnterRunsSelectedCommand(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})

	m, _ = update(t, m, keyRunes("/"))
	m, _ = update(t, m, keyEnter())

	if string(m.input) != "" {
		t.Fatalf("input after selected slash command = %q, want empty", string(m.input))
	}
	if !entriesContain(m.entries, "system", "### Session") {
		t.Fatalf("enter did not execute selected /session command: %#v", m.entries)
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
	if suggestionCommandStyle.GetForeground() != lipgloss.Color("252") {
		t.Fatalf("non-selected slash command foreground = %q, want white", suggestionCommandStyle.GetForeground())
	}
	if suggestionDescriptionStyle.GetForeground() != lipgloss.Color("252") {
		t.Fatalf("non-selected slash description foreground = %q, want white", suggestionDescriptionStyle.GetForeground())
	}
	if suggestionSelectedStyle.GetForeground() != lipgloss.Color("81") {
		t.Fatalf("selected slash command foreground = %q, want blue", suggestionSelectedStyle.GetForeground())
	}

	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	m, _ = update(t, m, keyRunes("/"))
	rendered := m.renderSlashSuggestions(72)
	if strings.Contains(rendered, "\x1b[48;") {
		t.Fatalf("slash dropdown rendered background color escapes, want foreground colors only:\n%s", rendered)
	}
}

func TestModelShiftTabTogglesPlanMode(t *testing.T) {
	runner := newFakeRunner()
	m := NewModel(context.Background(), runner, Config{})
	if m.planMode {
		t.Fatalf("initial planMode = true, want false")
	}
	if strings.Contains(m.View(), "plan mode on") || strings.Contains(m.View(), "plan off") {
		t.Fatalf("plan mode off should not render a plan label:\n%s", m.View())
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
	headerLines := strings.Split(stripANSI(m.renderHeader(100)), "\n")
	if len(headerLines) < 2 {
		t.Fatalf("plan mode should render on a new header line:\n%s", m.renderHeader(100))
	}
	if strings.Contains(headerLines[0], "plan mode on") {
		t.Fatalf("plan mode rendered on the status line:\n%s", m.renderHeader(100))
	}
	if !strings.Contains(headerLines[1], "plan mode on") {
		t.Fatalf("plan mode missing from second header line:\n%s", m.renderHeader(100))
	}

	m, _ = update(t, m, keyShiftTab())
	if m.planMode || runner.planMode {
		t.Fatalf("plan mode was not disabled: model=%v runner=%v", m.planMode, runner.planMode)
	}
	if m.status != "Plan mode disabled" {
		t.Fatalf("status = %q, want Plan mode disabled", m.status)
	}
	if strings.Contains(m.View(), "plan mode on") || strings.Contains(m.View(), "plan off") {
		t.Fatalf("plan mode off should not render a plan label after disabling:\n%s", m.View())
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
	for _, want := range []string{"FOXHARNESS", "[fake-model] work", "git:(-)", "Context:", "░░░░░░░░░░ 7%", "sid:sess-1", "Message foxharness"} {
		if !strings.Contains(plainView, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Index(plainView, "Message foxharness") > strings.Index(plainView, "FOXHARNESS") {
		t.Fatalf("header should render below the input box:\n%s", view)
	}
	if strings.Contains(view, "plan mode on") || strings.Contains(view, "plan off") {
		t.Fatalf("plan mode off should not render plan label:\n%s", view)
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
	if !strings.Contains(view, "• Working (58s • esc to interrupt)") {
		t.Fatalf("view missing running notice:\n%s", view)
	}
	if strings.Contains(view, "> • Working") {
		t.Fatalf("running notice rendered inside input:\n%s", view)
	}

	noticeIndex := strings.Index(view, "Working (58s")
	inputIndex := strings.Index(view, "> Input locked until the current run completes.")
	if inputIndex < 0 {
		t.Fatalf("view missing locked input text:\n%s", view)
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

func keyCtrlC() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyCtrlC}
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
