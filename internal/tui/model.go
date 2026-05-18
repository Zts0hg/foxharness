package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	minWidth  = 72
	minHeight = 20

	quitConfirmWindow = 2 * time.Second
	runningTickEvery  = time.Second
)

// Runner is the app-facing runtime required by the TUI. It is intentionally
// small so tests can exercise the UI without calling a real model.
type Runner interface {
	Run(ctx context.Context, prompt string, reporter engine.Reporter) (*engine.RunResult, error)
	NewSession(ctx context.Context) (string, error)
	SessionID() string
	SessionDir() string
	WorkDir() string
	Model() string
	ContextUsage() string
	MessageHistory() ([]session.MessageRecord, error)
	PlanMode() bool
	SetPlanMode(enabled bool)
}

// Config controls the initial TUI presentation.
type Config struct {
	Model         string
	InitialPrompt string
}

// Run starts the interactive chat TUI.
func Run(ctx context.Context, runner Runner, cfg Config) error {
	m := NewModel(ctx, runner, cfg)
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithContext(ctx)).Run()
	return err
}

type entry struct {
	role  string
	title string
	body  string
	err   bool
	time  time.Time
}

type slashCommand struct {
	Name        string
	Description string
}

var slashCommands = []slashCommand{
	{Name: "/session", Description: "show current session paths"},
	{Name: "/clear", Description: "clear the visible transcript"},
	{Name: "/new", Description: "start a fresh session"},
	{Name: "/cancel", Description: "cancel the active run"},
	{Name: "/help", Description: "show available commands"},
	{Name: "/exit", Description: "quit"},
}

var workingFrames = []string{"•", "◦", "●", "◌"}

type Model struct {
	ctx    context.Context
	runner Runner
	events chan tea.Msg
	now    func() time.Time

	width  int
	height int

	input          []rune
	inputHistory   []string
	historyIndex   int
	historyDraft   []rune
	slashSelection int
	fileSelection  int
	fileMentions   []fileMention
	queuedPrompts  []string

	entries      []entry
	status       string
	running      bool
	runStartedAt time.Time
	spinnerFrame int
	scrollOffset int
	cancelRun    context.CancelFunc
	lastCtrlC    time.Time

	sessionID    string
	modelName    string
	project      string
	gitBranch    string
	contextUsage string
	planMode     bool
}

func NewModel(ctx context.Context, runner Runner, cfg Config) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	modelName := cfg.Model
	if modelName == "" {
		modelName = runner.Model()
	}
	entries, inputHistory, status := initialSessionState(runner)
	return Model{
		ctx:            ctx,
		runner:         runner,
		events:         make(chan tea.Msg, 256),
		now:            time.Now,
		width:          96,
		height:         28,
		input:          []rune(cfg.InitialPrompt),
		inputHistory:   inputHistory,
		historyIndex:   -1,
		slashSelection: -1,
		fileSelection:  -1,
		fileMentions:   loadFileMentions(runner.WorkDir()),
		status:         status,
		sessionID:      runner.SessionID(),
		modelName:      modelName,
		project:        projectFolderName(runner.WorkDir()),
		gitBranch:      gitBranchForWorkDir(runner.WorkDir()),
		contextUsage:   normalizeContextUsage(runner.ContextUsage()),
		planMode:       runner.PlanMode(),
		entries:        entries,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(waitForRunEvent(m.ctx, m.events), runningTickCmd())
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, minWidth)
		m.height = max(msg.Height, minHeight)
		return m, nil

	case runEventMsg:
		m.applyRunEvent(msg)
		return m, waitForRunEvent(m.ctx, m.events)

	case runFinishedMsg:
		m.drainRunEvents()
		m.running = false
		m.runStartedAt = time.Time{}
		m.cancelRun = nil
		m.refreshRuntimeInfo()
		if msg.err != nil {
			m.status = "Run failed"
			if len(m.queuedPrompts) > 0 {
				m.status = fmt.Sprintf("Run failed; %d queued", len(m.queuedPrompts))
			}
			if !m.lastEntryContainsError(msg.err) {
				m.appendEntry("error", "run failed", msg.err.Error(), true)
			}
			if len(m.queuedPrompts) > 0 {
				return m.startNextQueuedPrompt()
			}
			return m, nil
		}
		if msg.result != nil {
			m.status = fmt.Sprintf("Run complete: %s", msg.result.RunID)
		} else {
			m.status = "Run complete"
		}
		if len(m.queuedPrompts) > 0 {
			return m.startNextQueuedPrompt()
		}
		return m, nil

	case newSessionFinishedMsg:
		m.running = false
		m.runStartedAt = time.Time{}
		m.cancelRun = nil
		if msg.err != nil {
			m.status = "New session failed"
			m.appendEntry("error", "new session failed", msg.err.Error(), true)
			return m, nil
		}
		m.sessionID = msg.sessionID
		m.refreshRuntimeInfo()
		m.status = "New session ready"
		m.entries = nil
		m.inputHistory = nil
		m.queuedPrompts = nil
		m.resetHistoryNavigation()
		m.scrollOffset = 0
		m.appendCommandEntry("New session", formatSessionRows(
			msg.sessionID,
			m.runner.SessionDir(),
			m.runner.WorkDir(),
			m.runner.Model(),
		))
		return m, nil

	case runningTickMsg:
		if m.running {
			m.spinnerFrame++
		}
		return m, runningTickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.scrollOffset += scrollDelta("wheelup")
	case tea.MouseButtonWheelDown:
		m.scrollOffset -= scrollDelta("wheeldown")
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key != "ctrl+c" {
		m.lastCtrlC = time.Time{}
	}

	switch key {
	case "ctrl+c":
		now := m.nowTime()
		if !m.lastCtrlC.IsZero() && now.Sub(m.lastCtrlC) <= quitConfirmWindow {
			if m.cancelRun != nil {
				m.cancelRun()
			}
			return m, tea.Quit
		}
		m.lastCtrlC = now
		m.status = "Press Ctrl+C again within 2s to quit"
		return m, nil
	case "esc":
		if m.running && m.cancelRun != nil {
			m.cancelRun()
			m.status = "Cancel requested"
			m.appendEntry("system", "cancel", "Current run cancellation requested.", false)
			return m, nil
		}
		m.input = nil
		m.resetHistoryNavigation()
		m.resetCompletions()
		return m, nil
	case "enter":
		return m.submitInput()
	case "shift+tab":
		return m.togglePlanMode()
	case "tab":
		if m.hasSlashMenu() {
			m.completeSlashCommand()
		} else if m.hasFileMentionMenu() {
			m.completeFileMention()
		}
		return m, nil
	case "backspace", "ctrl+h":
		if len(m.input) > 0 {
			m.resetHistoryNavigation()
			m.input = m.input[:len(m.input)-1]
			m.updateCompletions()
		}
		return m, nil
	case " ":
		m.resetHistoryNavigation()
		m.input = append(m.input, ' ')
		m.updateCompletions()
		return m, nil
	case "ctrl+u":
		m.input = nil
		m.resetHistoryNavigation()
		m.resetCompletions()
		return m, nil
	case "up":
		if m.hasSlashMenu() {
			m.moveSlashSelection(-1)
			return m, nil
		}
		if m.hasFileMentionMenu() {
			m.moveFileSelection(-1)
			return m, nil
		}
		m.recallPreviousInput()
		return m, nil
	case "down":
		if m.hasSlashMenu() {
			m.moveSlashSelection(1)
			return m, nil
		}
		if m.hasFileMentionMenu() {
			m.moveFileSelection(1)
			return m, nil
		}
		m.recallNextInput()
		return m, nil
	case "pgup":
		m.scrollOffset += scrollDelta(msg.String())
		return m, nil
	case "pgdown":
		m.scrollOffset -= scrollDelta(msg.String())
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		return m, nil
	case "end":
		m.scrollOffset = 0
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.resetHistoryNavigation()
		m.input = append(m.input, msg.Runes...)
		m.updateCompletions()
	}
	return m, nil
}

func (m Model) togglePlanMode() (tea.Model, tea.Cmd) {
	m.planMode = !m.planMode
	m.runner.SetPlanMode(m.planMode)
	if m.planMode {
		if m.running {
			m.status = "Plan mode enabled for next run"
		} else {
			m.status = "Plan mode enabled"
		}
	} else {
		if m.running {
			m.status = "Plan mode disabled for next run"
		} else {
			m.status = "Plan mode disabled"
		}
	}
	return m, nil
}

func (m *Model) completeSlashCommand() {
	command, ok := m.selectedSlashCommand()
	if !ok {
		return
	}
	m.input = []rune(command.Name)
	m.updateCompletions()
}

func (m *Model) completeFileMention() {
	mention, ok := m.selectedFileMention()
	if !ok {
		return
	}
	start, end, _, ok := m.activeFileMention()
	if !ok {
		return
	}
	replacement := []rune("@" + mention.Path)
	next := make([]rune, 0, len(m.input)-end+start+len(replacement)+1)
	next = append(next, m.input[:start]...)
	next = append(next, replacement...)
	if end == len(m.input) {
		next = append(next, ' ')
	} else {
		next = append(next, m.input[end:]...)
	}
	m.input = next
	m.updateCompletions()
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(string(m.input))
	if text == "" {
		return m, nil
	}
	if strings.HasPrefix(text, "/") {
		if command, ok := m.selectedSlashCommand(); ok {
			text = command.Name
		}
		m.input = nil
		m.resetHistoryNavigation()
		m.resetCompletions()
		return m.handleSlashCommand(text)
	}
	if m.running {
		m.addInputHistory(text)
		m.input = nil
		m.resetHistoryNavigation()
		m.resetCompletions()
		m.queuedPrompts = append(m.queuedPrompts, text)
		m.status = fmt.Sprintf("Queued %d message%s", len(m.queuedPrompts), pluralS(len(m.queuedPrompts)))
		return m, nil
	}

	m.addInputHistory(text)
	m.input = nil
	m.resetHistoryNavigation()
	m.resetCompletions()
	return m.startPrompt(text)
}

func (m Model) startPrompt(text string) (tea.Model, tea.Cmd) {
	m.scrollOffset = 0
	m.running = true
	m.runStartedAt = m.nowTime()
	m.spinnerFrame = 0
	m.status = "Running"
	m.appendEntry("user", "you", text, false)

	runCtx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel
	return m, runPromptCmd(runCtx, m.runner, text, m.events)
}

func (m Model) startNextQueuedPrompt() (tea.Model, tea.Cmd) {
	if len(m.queuedPrompts) == 0 {
		return m, nil
	}
	text := m.queuedPrompts[0]
	m.queuedPrompts = append([]string(nil), m.queuedPrompts[1:]...)
	next, cmd := m.startPrompt(text)
	typed := next.(Model)
	if len(typed.queuedPrompts) > 0 {
		typed.status = fmt.Sprintf("Running queued message; %d queued", len(typed.queuedPrompts))
	} else {
		typed.status = "Running queued message"
	}
	return typed, cmd
}

func (m *Model) addInputHistory(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if len(m.inputHistory) > 0 && m.inputHistory[len(m.inputHistory)-1] == text {
		return
	}
	m.inputHistory = append(m.inputHistory, text)
}

func (m *Model) recallPreviousInput() {
	if len(m.inputHistory) == 0 {
		return
	}
	if m.historyIndex == -1 {
		m.historyDraft = append([]rune(nil), m.input...)
		m.historyIndex = len(m.inputHistory) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}
	m.input = []rune(m.inputHistory[m.historyIndex])
}

func (m *Model) recallNextInput() {
	if m.historyIndex == -1 {
		return
	}
	if m.historyIndex < len(m.inputHistory)-1 {
		m.historyIndex++
		m.input = []rune(m.inputHistory[m.historyIndex])
		return
	}
	m.historyIndex = -1
	m.input = append([]rune(nil), m.historyDraft...)
	m.historyDraft = nil
}

func (m *Model) resetHistoryNavigation() {
	m.historyIndex = -1
	m.historyDraft = nil
}

func (m *Model) updateSlashSelection() {
	matches := m.matchingSlashCommands()
	if len(matches) == 0 {
		m.resetSlashSelection()
		return
	}
	if m.slashSelection < 0 || m.slashSelection >= len(matches) {
		m.slashSelection = 0
	}
}

func (m *Model) resetSlashSelection() {
	m.slashSelection = -1
}

func (m *Model) updateFileSelection() {
	matches := m.matchingFileMentions()
	if len(matches) == 0 {
		m.resetFileSelection()
		return
	}
	if m.fileSelection < 0 || m.fileSelection >= len(matches) {
		m.fileSelection = 0
	}
}

func (m *Model) resetFileSelection() {
	m.fileSelection = -1
}

func (m *Model) updateCompletions() {
	m.updateSlashSelection()
	m.updateFileSelection()
}

func (m *Model) resetCompletions() {
	m.resetSlashSelection()
	m.resetFileSelection()
}

func (m Model) hasSlashMenu() bool {
	return len(m.matchingSlashCommands()) > 0
}

func (m Model) hasFileMentionMenu() bool {
	return len(m.matchingFileMentions()) > 0
}

func (m *Model) moveSlashSelection(delta int) {
	matches := m.matchingSlashCommands()
	if len(matches) == 0 {
		m.resetSlashSelection()
		return
	}
	if m.slashSelection < 0 || m.slashSelection >= len(matches) {
		m.slashSelection = 0
	}
	m.slashSelection = (m.slashSelection + delta + len(matches)) % len(matches)
}

func (m *Model) moveFileSelection(delta int) {
	matches := m.matchingFileMentions()
	if len(matches) == 0 {
		m.resetFileSelection()
		return
	}
	if m.fileSelection < 0 || m.fileSelection >= len(matches) {
		m.fileSelection = 0
	}
	m.fileSelection = (m.fileSelection + delta + len(matches)) % len(matches)
}

func (m Model) selectedSlashCommand() (slashCommand, bool) {
	matches := m.matchingSlashCommands()
	if len(matches) == 0 {
		return slashCommand{}, false
	}
	index := m.slashSelection
	if index < 0 || index >= len(matches) {
		index = 0
	}
	return matches[index], true
}

func (m Model) selectedFileMention() (fileMention, bool) {
	matches := m.matchingFileMentions()
	if len(matches) == 0 {
		return fileMention{}, false
	}
	index := m.fileSelection
	if index < 0 || index >= len(matches) {
		index = 0
	}
	return matches[index], true
}

func (m Model) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	switch cmd {
	case "/help":
		m.appendCommandEntry("Commands", slashCommandHelp())
		m.status = "Help"
		return m, nil
	case "/session":
		m.appendCommandEntry("Session", formatSessionRows(
			m.runner.SessionID(),
			m.runner.SessionDir(),
			m.runner.WorkDir(),
			m.runner.Model(),
		))
		m.status = "Session details"
		return m, nil
	case "/clear":
		m.entries = nil
		m.status = "Transcript cleared"
		m.scrollOffset = 0
		return m, nil
	case "/new":
		if m.running {
			m.status = "Cannot create a new session while a run is active"
			return m, nil
		}
		m.running = true
		m.runStartedAt = m.nowTime()
		m.spinnerFrame = 0
		m.status = "Creating new session"
		return m, newSessionCmd(m.ctx, m.runner)
	case "/cancel":
		if m.cancelRun == nil {
			m.status = "No active run"
			return m, nil
		}
		m.cancelRun()
		m.status = "Cancel requested"
		m.appendCommandEntry("Cancel", "Current run cancellation requested.")
		return m, nil
	case "/exit", "/quit":
		return m, tea.Quit
	default:
		m.appendEntry("error", "unknown command", fmt.Sprintf("Unknown command: %s", cmd), true)
		m.status = "Unknown command"
		return m, nil
	}
}

func (m Model) matchingSlashCommands() []slashCommand {
	text := strings.TrimSpace(string(m.input))
	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \t\n") {
		return nil
	}
	var matches []slashCommand
	for _, command := range slashCommands {
		if text == "/" || strings.HasPrefix(command.Name, text) {
			matches = append(matches, command)
		}
	}
	return matches
}

func (m Model) matchingFileMentions() []fileMention {
	if m.hasSlashMenu() {
		return nil
	}
	_, _, query, ok := m.activeFileMention()
	if !ok {
		return nil
	}
	return matchFileMentions(m.fileMentions, query)
}

func slashCommandHelp() string {
	commandWidth := 0
	for _, command := range slashCommands {
		if len(command.Name) > commandWidth {
			commandWidth = len(command.Name)
		}
	}
	lines := make([]string, 0, len(slashCommands))
	for _, command := range slashCommands {
		lines = append(lines, fmt.Sprintf("%-*s  %s", commandWidth, command.Name, command.Description))
	}
	return strings.Join(lines, "\n")
}

func formatSessionRows(sessionID, sessionDir, workDir, model string) string {
	rows := []struct {
		label string
		value string
	}{
		{label: "ID", value: sessionID},
		{label: "Dir", value: sessionDir},
		{label: "Workdir", value: workDir},
		{label: "Model", value: model},
	}
	labelWidth := 0
	for _, row := range rows {
		if len(row.label) > labelWidth {
			labelWidth = len(row.label)
		}
	}
	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lines = append(lines, fmt.Sprintf("%-*s  %s", labelWidth, row.label, row.value))
	}
	return strings.Join(lines, "\n")
}

func initialSessionState(runner Runner) ([]entry, []string, string) {
	records, err := runner.MessageHistory()
	if err != nil {
		return []entry{
			sessionStartedEntry(),
			{
				role:  "error",
				title: "history",
				body:  fmt.Sprintf("Failed to load session history: %v", err),
				err:   true,
				time:  time.Now(),
			},
		}, nil, "History load failed"
	}

	entries := entriesFromMessageHistory(records)
	if len(entries) == 0 {
		return []entry{sessionStartedEntry()}, nil, "Ready"
	}
	return entries, inputHistoryFromMessageHistory(records), "Resumed session: " + runner.SessionID()
}

func sessionStartedEntry() entry {
	return entry{
		role:  "system",
		title: "session",
		body:  "Interactive session started. Type /help for commands.",
		time:  time.Now(),
	}
}

func entriesFromMessageHistory(records []session.MessageRecord) []entry {
	entries := make([]entry, 0, len(records))
	toolNames := make(map[string]string)
	for _, record := range records {
		msg := record.Message
		when := historyEntryTime(record.Time)
		switch {
		case msg.Role == schema.RoleUser && msg.ToolCallID == "":
			if !isRenderableHistoryContent(msg.Content) {
				continue
			}
			entries = append(entries, entry{
				role:  "user",
				title: "you",
				body:  msg.Content,
				time:  when,
			})
		case msg.Role == schema.RoleAssistant:
			if strings.TrimSpace(msg.Content) != "" {
				entries = append(entries, entry{
					role:  "assistant",
					title: "foxharness",
					body:  msg.Content,
					time:  when,
				})
			}
			for _, call := range msg.ToolCalls {
				toolNames[call.ID] = call.Name
				entries = append(entries, entry{
					role:  "tool",
					title: "call " + call.Name,
					body:  formatToolInvocation(call.Name, string(call.Arguments)),
					time:  when,
				})
			}
		case msg.ToolCallID != "":
			toolName := toolNames[msg.ToolCallID]
			if toolName == "" {
				toolName = "tool"
			}
			entries = append(entries, entry{
				role:  "tool",
				title: "result " + toolName,
				body:  msg.Content,
				time:  when,
			})
		}
	}
	return entries
}

func inputHistoryFromMessageHistory(records []session.MessageRecord) []string {
	history := make([]string, 0, len(records))
	for _, record := range records {
		msg := record.Message
		if msg.Role != schema.RoleUser || msg.ToolCallID != "" || !isRenderableHistoryContent(msg.Content) {
			continue
		}
		text := strings.TrimSpace(msg.Content)
		if len(history) > 0 && history[len(history)-1] == text {
			continue
		}
		history = append(history, text)
	}
	return history
}

func isRenderableHistoryContent(content string) bool {
	content = strings.TrimSpace(content)
	return content != "" && !isCompactionSummaryMessage(content)
}

func isCompactionSummaryMessage(content string) bool {
	return strings.HasPrefix(strings.TrimSpace(content), "## Compacted Context Summary")
}

func historyEntryTime(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

func (m Model) nowTime() time.Time {
	if m.now == nil {
		return time.Now()
	}
	return m.now()
}

func (m *Model) appendEntry(role, title, body string, isError bool) {
	m.entries = append(m.entries, entry{
		role:  role,
		title: title,
		body:  strings.TrimSpace(body),
		err:   isError,
		time:  time.Now(),
	})
}

func (m *Model) appendCommandEntry(title, body string) {
	m.appendEntry("command", title, body, false)
}

func (m Model) lastEntryContainsError(err error) bool {
	if err == nil || len(m.entries) == 0 {
		return false
	}
	last := m.entries[len(m.entries)-1]
	return last.err && strings.Contains(last.body, err.Error())
}

func (m *Model) drainRunEvents() {
	for {
		select {
		case msg := <-m.events:
			if event, ok := msg.(runEventMsg); ok {
				m.applyRunEvent(event)
			}
		default:
			return
		}
	}
}

func (m *Model) applyRunEvent(msg runEventMsg) {
	m.status = msg.status
	if msg.role != "" || msg.body != "" {
		m.appendEntry(msg.role, msg.title, msg.body, msg.err)
	}
}

func (m Model) workingFrame() string {
	if len(workingFrames) == 0 {
		return "•"
	}
	return workingFrames[m.spinnerFrame%len(workingFrames)]
}

func (m Model) runningElapsed() time.Duration {
	if m.runStartedAt.IsZero() {
		return 0
	}
	elapsed := m.nowTime().Sub(m.runStartedAt)
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func scrollDelta(key string) int {
	if key == "pgup" || key == "pgdown" {
		return 8
	}
	return 1
}

func pluralS(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func waitForRunEvent(ctx context.Context, events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-events:
			return msg
		case <-ctx.Done():
			return tea.Quit()
		}
	}
}

type runningTickMsg struct{}

func runningTickCmd() tea.Cmd {
	return tea.Tick(runningTickEvery, func(time.Time) tea.Msg {
		return runningTickMsg{}
	})
}

type runEventMsg struct {
	role   string
	title  string
	body   string
	status string
	err    bool
}

type runFinishedMsg struct {
	result *engine.RunResult
	err    error
}

type newSessionFinishedMsg struct {
	sessionID string
	err       error
}

func runPromptCmd(ctx context.Context, runner Runner, prompt string, events chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		reporter := &channelReporter{events: events}
		result, err := runner.Run(ctx, prompt, reporter)
		return runFinishedMsg{result: result, err: err}
	}
}

func newSessionCmd(ctx context.Context, runner Runner) tea.Cmd {
	return func() tea.Msg {
		sessionID, err := runner.NewSession(ctx)
		return newSessionFinishedMsg{sessionID: sessionID, err: err}
	}
}
