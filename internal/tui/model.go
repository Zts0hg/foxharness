package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/engine"
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
}

// Config controls the initial TUI presentation.
type Config struct {
	Model         string
	InitialPrompt string
}

// Run starts the interactive chat TUI.
func Run(ctx context.Context, runner Runner, cfg Config) error {
	m := NewModel(ctx, runner, cfg)
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
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

	input []rune

	entries      []entry
	status       string
	running      bool
	runStartedAt time.Time
	spinnerFrame int
	scrollOffset int
	cancelRun    context.CancelFunc
	lastCtrlC    time.Time

	sessionID string
	modelName string
}

func NewModel(ctx context.Context, runner Runner, cfg Config) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	modelName := cfg.Model
	if modelName == "" {
		modelName = runner.Model()
	}
	return Model{
		ctx:       ctx,
		runner:    runner,
		events:    make(chan tea.Msg, 256),
		now:       time.Now,
		width:     96,
		height:    28,
		input:     []rune(cfg.InitialPrompt),
		status:    "Ready",
		sessionID: runner.SessionID(),
		modelName: modelName,
		entries: []entry{{
			role:  "system",
			title: "session",
			body:  "Interactive session started. Type /help for commands.",
			time:  time.Now(),
		}},
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
		if msg.err != nil {
			m.status = "Run failed"
			m.appendEntry("error", "run failed", msg.err.Error(), true)
			return m, nil
		}
		if msg.result != nil {
			m.status = fmt.Sprintf("Run complete: %s", msg.result.RunID)
		} else {
			m.status = "Run complete"
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
		m.status = "New session ready"
		m.appendEntry("system", "new session", fmt.Sprintf("Switched to session %s", msg.sessionID), false)
		return m, nil

	case runningTickMsg:
		if m.running {
			m.spinnerFrame++
		}
		return m, runningTickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
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
		return m, nil
	case "enter":
		return m.submitInput()
	case "tab":
		if !m.running {
			m.completeSlashCommand()
		}
		return m, nil
	case "backspace", "ctrl+h":
		if !m.running && len(m.input) > 0 {
			m.input = m.input[:len(m.input)-1]
		}
		return m, nil
	case " ":
		if !m.running {
			m.input = append(m.input, ' ')
		}
		return m, nil
	case "ctrl+u":
		if !m.running {
			m.input = nil
		}
		return m, nil
	case "up", "pgup":
		m.scrollOffset += scrollDelta(msg.String())
		return m, nil
	case "down", "pgdown":
		m.scrollOffset -= scrollDelta(msg.String())
		if m.scrollOffset < 0 {
			m.scrollOffset = 0
		}
		return m, nil
	case "end":
		m.scrollOffset = 0
		return m, nil
	}

	if m.running {
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.input = append(m.input, msg.Runes...)
	}
	return m, nil
}

func (m *Model) completeSlashCommand() {
	matches := m.matchingSlashCommands()
	if len(matches) == 0 {
		return
	}
	m.input = []rune(matches[0].Name)
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(string(m.input))
	if text == "" {
		return m, nil
	}
	if strings.HasPrefix(text, "/") {
		m.input = nil
		return m.handleSlashCommand(text)
	}
	if m.running {
		m.status = "A run is already active"
		return m, nil
	}

	m.input = nil
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

func (m Model) handleSlashCommand(text string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	switch cmd {
	case "/help":
		m.appendEntry("system", "commands", slashCommandHelp(), false)
		m.status = "Help"
		return m, nil
	case "/session":
		m.appendEntry("system", "session", fmt.Sprintf(
			"Session: %s\nSession Dir: %s\nWorkdir: %s\nModel: %s",
			m.runner.SessionID(),
			m.runner.SessionDir(),
			m.runner.WorkDir(),
			m.runner.Model(),
		), false)
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
		m.appendEntry("system", "cancel", "Current run cancellation requested.", false)
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
	if m.running {
		return nil
	}
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

func slashCommandHelp() string {
	lines := make([]string, 0, len(slashCommands))
	for _, command := range slashCommands {
		lines = append(lines, fmt.Sprintf("%-9s %s", command.Name, command.Description))
	}
	return strings.Join(lines, "\n")
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
