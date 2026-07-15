package tui

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/effort"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/schema"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/settings"
	"github.com/Zts0hg/foxharness/internal/slash"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tui/selector"
	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
)

const (
	minWidth  = 60
	minHeight = 20

	quitConfirmWindow = 2 * time.Second
	runningTickEvery  = 150 * time.Millisecond
	pendingEscDelay   = 50 * time.Millisecond
	mouseTailDelay    = 150 * time.Millisecond
	inputHistoryLimit = 100

	maxShellCommandOutputBytes = tools.MaxBashOutputBytes
)

// Runner is the app-facing runtime required by the TUI. It is intentionally
// small so tests can exercise the UI without calling a real model.
type Runner interface {
	RunInCollaborationMode(ctx context.Context, prompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error)
	NewSession(ctx context.Context) (string, error)
	SessionID() string
	SessionDir() string
	WorkDir() string
	// AutoMemoryIndex returns the merged two-tier persistent memory index
	// (descriptions only) for sidebar display, or "" when no store is wired.
	AutoMemoryIndex() string
	Model() string
	SetModel(model string) error
	ContextUsage() string
	MessageHistory() ([]session.MessageRecord, error)
	TruncateMessageHistory(seq int64) error
	RestoreSessionStateBeforeMessage(seq int64) (bool, error)
	Checkpointer() checkpoint.Checkpointer
	CollaborationMode() collaboration.Mode
	SetCollaborationMode(mode collaboration.Mode)
	CompactNow(ctx context.Context, customInstructions string) (*compaction.CompactResult, error)
}

type permissionRuntime interface {
	PermissionSnapshot() permission.Snapshot
	SetPermissionMode(mode permission.Mode, remembered bool)
	ActivateFullAccess(remember bool)
	ClearPermissionGrants() int
}

type effortRuntime interface {
	SetEffortOverride(value string)
}

// Config controls the initial TUI presentation.
type Config struct {
	Model         string
	InitialPrompt string
	HomeDir       string
	// EffortOverride is the resolved session-level effort value that should
	// seed the /effort selector before the user changes it interactively.
	EffortOverride string

	ProviderID        string
	ProviderProfileID string
	ProviderProtocol  string

	// Registry, when non-nil, attaches a file-based slash command registry
	// to the TUI so that prompt commands from .foxharness/ and .claude/
	// command and skill directories appear alongside the built-ins.
	Registry *slash.Registry

	// Executor is the per-command pipeline used when a prompt command is
	// invoked. May be nil; a default executor with no fork runner is used
	// in that case.
	Executor *slash.Executor

	// Asker, when non-nil, enables the interactive ask_user_question overlay.
	// Only the TUI sets it; non-interactive runners leave it nil so the tool
	// is never offered to the model.
	Asker *Asker

	// PlanReviewer, when non-nil, enables submit_plan confirmation and revision.
	PlanReviewer *PlanReviewer
	Permissions  *PermissionBridge

	// Autodev, when non-nil, launches the backlog autopilot for the
	// /autodev builtin. internal/app injects it so the tui -> autodev
	// dependency stays one-way.
	Autodev func(ctx context.Context, backlogPath string, reporter autodev.Reporter) error
}

// Run starts the interactive chat TUI.
func Run(ctx context.Context, runner Runner, cfg Config) error {
	m := NewModel(ctx, runner, cfg)
	if cfg.Registry != nil {
		m = m.WithRegistry(cfg.Registry, cfg.Executor)
	}
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus(), tea.WithContext(ctx)).Run()
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
	Name         string
	Description  string
	Arguments    string
	ArgumentHint string
}

func (c slashCommand) hasArgumentHint() bool {
	return c.Arguments != "" || c.ArgumentHint != ""
}

type escAction string

const (
	escActionNone   escAction = ""
	escActionClear  escAction = "clear"
	escActionRewind escAction = "rewind"
)

var slashCommands = []slashCommand{
	{Name: "/status", Description: "show session status overview"},
	{Name: "/session", Description: "alias for /status"},
	{Name: "/clear", Description: "alias for /new"},
	{Name: "/rewind", Description: "restore a previous checkpoint"},
	{Name: "/checkpoint", Description: "alias for /rewind"},
	{Name: "/new", Description: "start a fresh session"},
	{Name: "/plan", Description: "enter or leave Formal Plan mode", ArgumentHint: "[off]"},
	{Name: "/model", Description: "show or switch the active model"},
	{Name: "/theme", Description: "show or switch the TUI theme", ArgumentHint: "[name]"},
	{Name: "/statusline", Description: "configure statusline items", ArgumentHint: "[set <items>|default]"},
	{Name: "/permissions", Description: "configure tool approval mode"},
	{Name: "/effort", Description: "configure reasoning effort"},
	{Name: "/compact", Description: "compact context", ArgumentHint: "<optional custom instructions>"},
	{Name: "/autodev", Description: "drain the backlog autonomously", ArgumentHint: "[backlog-path]"},
	{Name: "/cancel", Description: "cancel the active run"},
	{Name: "/sidebar", Description: "show or hide right sidebar"},
	{Name: "/help", Description: "show available commands"},
	{Name: "/exit", Description: "quit"},
}

var workingFrames = []string{"✦", "✧"}

const defaultThemeName = "codex"

var defaultStatuslineItems = []string{"model", "project", "git-branch", "context-used"}

type selectionPoint struct {
	line int
	col  int
}

type selectionArea string

const (
	selectionAreaTranscript selectionArea = "transcript"
	selectionAreaSidebar    selectionArea = "sidebar"
	selectionAreaInput      selectionArea = "input"
)

type selectionState struct {
	anchor   selectionPoint
	focus    selectionPoint
	active   bool
	dragging bool
	area     selectionArea
}

type inputPastePreview struct {
	id        int
	lineCount int
}

type queuedPrompt struct {
	text string
	mode collaboration.Mode
}

type Model struct {
	ctx           context.Context
	runner        Runner
	events        chan tea.Msg
	now           func() time.Time
	copySelection func(string) error

	width  int
	height int

	input              []rune
	inputCursor        int
	inputPastePreview  *inputPastePreview
	pastePreviewSeq    int
	inputHistory       []string
	historyIndex       int
	historyDraft       []rune
	historyDraftCursor int
	slashSelection     int
	fileSelection      int
	fileMentions       []fileMention
	queuedPrompts      []queuedPrompt

	entries            []entry
	status             string
	running            bool
	runStartedAt       time.Time
	spinnerFrame       int
	scrollOffset       int
	toolOutputExpanded bool

	cachedLayout  *transcriptLayout
	lastWheelTime time.Time
	wheelSpeed    int

	cancelRun     context.CancelFunc
	lastCtrlC     time.Time
	lastEsc       time.Time
	lastEscAction escAction
	pendingEsc    time.Time
	pendingEscID  uint64
	mouseTail     []rune
	mouseTailEsc  bool
	mouseTailID   uint64
	selection     selectionState

	sessionID         string
	modelName         string
	project           string
	gitBranch         string
	contextUsage      string
	collaborationMode collaboration.Mode
	homeDir           string

	providerID        string
	providerProfileID string
	providerProtocol  string

	themeName       string
	statuslineItems []string

	checkpointer   checkpoint.Checkpointer
	rewindSelector *selector.Model

	asker   *Asker
	askForm *askForm

	planReviewer       *PlanReviewer
	planForm           *planReviewForm
	permissionBridge   *PermissionBridge
	approvalForm       *approvalForm
	permissionForm     *permissionForm
	permissionSnapshot permission.Snapshot
	effortForm         *effortForm
	effortValue        string

	sidebarVisible       bool
	sidebarFocused       bool
	terminalFocused      bool
	sidebarFocusIndex    int
	sidebarDocuments     []sidebarDocument
	sidebarScrollOffsets [sidebarDocumentCount]int

	slashRegistry *slash.Registry
	slashExecutor *slash.Executor

	autodevLauncher func(ctx context.Context, backlogPath string, reporter autodev.Reporter) error
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
	themeName := defaultThemeName
	statuslineItems := append([]string(nil), defaultStatuslineItems...)
	effortValue := effort.Auto
	if strings.TrimSpace(cfg.HomeDir) != "" {
		if loaded, err := settings.Load(cfg.HomeDir); err == nil {
			if isBuiltInTheme(loaded.TUI.Theme) {
				themeName = normalizeThemeName(loaded.TUI.Theme)
			}
			statuslineItems = normalizeSavedStatuslineItems(loaded.TUI.Statusline)
			if value := strings.TrimSpace(loaded.LLM.Effort[strings.ToLower(strings.TrimSpace(cfg.ProviderProtocol))]); value != "" {
				effortValue = value
			}
		}
	}
	if value := strings.TrimSpace(cfg.EffortOverride); value != "" {
		if normalized, err := effort.Validate(cfg.ProviderProtocol, value); err == nil {
			effortValue = normalized
		}
	}
	themeName = applyTheme(themeName)
	permissionState := permissionSnapshot(runner)
	if permissionState.FullAccessNeedsWarning {
		entries = append(entries, entry{
			role:  "system",
			title: "permissions",
			body:  "Full Access is selected but not remembered. Effective mode is Ask for approval until you run /permissions full-access or /permissions full-access remember.",
			time:  time.Now(),
		})
	}
	return Model{
		ctx:                ctx,
		runner:             runner,
		events:             make(chan tea.Msg, 256),
		now:                time.Now,
		copySelection:      copyToClipboard,
		width:              96,
		height:             28,
		input:              []rune(cfg.InitialPrompt),
		inputCursor:        len([]rune(cfg.InitialPrompt)),
		inputHistory:       inputHistory,
		historyIndex:       -1,
		slashSelection:     -1,
		fileSelection:      -1,
		fileMentions:       loadFileMentions(runner.WorkDir()),
		status:             status,
		homeDir:            cfg.HomeDir,
		providerID:         cfg.ProviderID,
		providerProfileID:  cfg.ProviderProfileID,
		providerProtocol:   cfg.ProviderProtocol,
		effortValue:        effortValue,
		themeName:          themeName,
		statuslineItems:    statuslineItems,
		sessionID:          runner.SessionID(),
		modelName:          modelName,
		project:            projectFolderName(runner.WorkDir()),
		gitBranch:          gitBranchForWorkDir(runner.WorkDir()),
		contextUsage:       normalizeContextUsage(runner.ContextUsage()),
		collaborationMode:  collaboration.Normalize(runner.CollaborationMode()),
		checkpointer:       runner.Checkpointer(),
		asker:              cfg.Asker,
		planReviewer:       cfg.PlanReviewer,
		permissionBridge:   cfg.Permissions,
		permissionSnapshot: permissionState,
		autodevLauncher:    cfg.Autodev,
		entries:            entries,
		sidebarVisible:     true,
		terminalFocused:    true,
		sidebarFocusIndex:  -1,
		sidebarDocuments:   loadSidebarDocuments(runner.WorkDir(), runner.SessionDir(), runner.AutoMemoryIndex()),
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{waitForRunEvent(m.ctx, m.events), runningTickCmd()}
	if m.permissionBridge != nil {
		m.permissionBridge.SetEvents(m.events)
		cmds = append(cmds, listenForPermissionRequest(m.ctx, m.permissionBridge))
	}
	if m.asker != nil {
		cmds = append(cmds, listenForAsk(m.ctx, m.asker))
	}
	if m.planReviewer != nil {
		cmds = append(cmds, listenForPlanReview(m.ctx, m.planReviewer))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, minWidth)
		m.height = max(msg.Height, 1)
		m.clearSelection()
		m.clampSidebarScrollOffsets()
		if !m.shouldRenderSidebar() {
			m.sidebarFocused = false
		}
		return m, nil

	case tea.FocusMsg:
		m.terminalFocused = true
		return m, nil

	case tea.BlurMsg:
		m.terminalFocused = false
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

	case selector.ResultMsg:
		return m.handleSelectorResult(msg)

	case askUserMsg:
		m.askForm = newAskForm(msg.req)
		return m, nil

	case askDoneMsg:
		return m.handleAskDone()

	case planReviewMsg:
		m.planForm = newPlanReviewForm(msg.req)
		return m, nil

	case planReviewDoneMsg:
		return m.handlePlanReviewDone()

	case permissionUserMsg:
		m.approvalForm = newApprovalForm(msg.req)
		return m, nil

	case permissionReviewMsg:
		m.status = msg.status
		return m, waitForRunEvent(m.ctx, m.events)

	case permissionStateChangedMsg:
		m.permissionSnapshot = permissionSnapshot(m.runner)
		return m, waitForRunEvent(m.ctx, m.events)

	case approvalDoneMsg:
		return m.handleApprovalDone()

	case permissionDoneMsg:
		return m.handlePermissionDone()

	case effortDoneMsg:
		return m.handleEffortDone()

	case promptCommandReadyMsg:
		return m.handlePromptCommandReady(msg)

	case compactFinishedMsg:
		m.running = false
		m.runStartedAt = time.Time{}
		m.cancelRun = nil
		if msg.err != nil {
			m.status = "Compact failed"
			m.appendEntry("error", "compact failed", msg.err.Error(), true)
			return m, nil
		}
		body := fmt.Sprintf(
			"Summarized %d messages.\nTokens: %d → %d (saved %d)",
			msg.result.MessagesSummarized,
			msg.result.PreTokens,
			msg.result.PostTokens,
			msg.result.PreTokens-msg.result.PostTokens,
		)
		m.appendCommandEntry("Compact", body)
		m.refreshRuntimeInfo()
		m.status = "Context compacted"
		return m, nil

	case shellCommandFinishedMsg:
		m.running = false
		m.runStartedAt = time.Time{}
		m.cancelRun = nil
		m.refreshRuntimeInfo()
		if msg.result.Err != nil {
			m.appendEntry("command", "Shell: !"+msg.command, formatShellCommandResult(msg.result), true)
			if msg.result.TimedOut {
				m.status = "Shell command timed out"
			} else {
				m.status = "Shell command failed"
			}
			if len(m.queuedPrompts) > 0 {
				return m.startNextQueuedPrompt()
			}
			return m, nil
		}
		m.appendCommandEntry("Shell: !"+msg.command, formatShellCommandResult(msg.result))
		m.status = "Shell command complete"
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
		m.sidebarDocuments = loadSidebarDocuments(m.runner.WorkDir(), m.runner.SessionDir(), m.runner.AutoMemoryIndex())
		m.clampSidebarScrollOffsets()
		m.status = "New session ready"
		m.entries = nil
		m.cachedLayout = nil
		m.inputHistory = projectInputHistoryOrFallback(m.runner, m.inputHistory)
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
		m.spinnerFrame++
		if m.spinnerFrame%4 == 0 {
			m.sidebarDocuments = loadSidebarDocuments(m.runner.WorkDir(), m.runner.SessionDir(), m.runner.AutoMemoryIndex())
		}
		m.clampSidebarScrollOffsets()
		return m, runningTickCmd()

	case pendingEscTimeoutMsg:
		if m.pendingEsc.IsZero() || m.pendingEscID != msg.id {
			return m, nil
		}
		m.pendingEsc = time.Time{}
		return m.applyEscKey()

	case mouseTailTimeoutMsg:
		if len(m.mouseTail) == 0 || m.mouseTailID != msg.id {
			return m, nil
		}
		return m.flushMouseTailAsInput()

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}

	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.cachedLayout == nil {
		_, bodyHeight := m.contentDimensions()
		layout := m.transcriptLayout(m.chatWidth(), bodyHeight)
		m.cachedLayout = &layout
	}
	if m.selection.dragging && !isWheelMouse(msg) {
		switch m.selection.area {
		case selectionAreaInput:
			next, _ := m.handleInputSelectionMouse(msg)
			return next, nil
		case selectionAreaSidebar:
			next, _ := m.handleSidebarSelectionMouse(msg)
			return next, nil
		}
		next, _ := m.handleTranscriptSelectionMouse(msg)
		return next, nil
	}

	if sidebarIndex, ok := m.sidebarIndexAt(msg.X, msg.Y); ok {
		var delta int
		m, delta = m.wheelScrollDelta()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.sidebarScrollOffsets[sidebarIndex] -= delta
		case tea.MouseButtonWheelDown:
			m.sidebarScrollOffsets[sidebarIndex] += delta
		}
		m.clampSidebarScrollOffsets()
		if next, ok := m.handleSidebarSelectionMouse(msg); ok {
			return next, nil
		}
		return m, nil
	}

	if next, ok := m.handleInputSelectionMouse(msg); ok {
		return next, nil
	}

	if next, ok := m.handleTranscriptSelectionMouse(msg); ok {
		return next, nil
	}

	{
		var delta int
		m, delta = m.wheelScrollDelta()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollOffset += delta
		case tea.MouseButtonWheelDown:
			m.scrollOffset -= delta
			if m.scrollOffset < 0 {
				m.scrollOffset = 0
			}
		}
	}
	return m, nil
}

func isWheelMouse(msg tea.MouseMsg) bool {
	return msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown
}

func (m Model) handleTranscriptSelectionMouse(msg tea.MouseMsg) (Model, bool) {
	if isWheelMouse(msg) {
		return m, false
	}

	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		point, ok := m.transcriptPointAt(msg.X, msg.Y, false)
		if !ok {
			m.clearSelection()
			return m, true
		}
		m.selection = selectionState{
			anchor:   point,
			focus:    point,
			active:   true,
			dragging: true,
			area:     selectionAreaTranscript,
		}
		return m, true

	case m.selection.dragging && msg.Action == tea.MouseActionMotion:
		point, ok := m.transcriptPointAt(msg.X, msg.Y, true)
		if !ok {
			return m, true
		}
		m.selection.focus = point
		return m, true

	case m.selection.dragging && msg.Action == tea.MouseActionRelease:
		point, ok := m.transcriptPointAt(msg.X, msg.Y, true)
		if ok {
			m.selection.focus = point
		}
		m.selection.dragging = false
		text := m.selectedText()
		if strings.TrimSpace(text) == "" {
			m.clearSelection()
			return m, true
		}
		if m.copySelection != nil {
			if err := m.copySelection(text); err != nil {
				m.status = "Copy failed: " + err.Error()
			} else {
				m.status = "Selection copied"
			}
		}
		return m, true
	}

	return m, false
}

func (m Model) handleSidebarSelectionMouse(msg tea.MouseMsg) (Model, bool) {
	if isWheelMouse(msg) {
		return m, false
	}

	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		point, ok := m.sidebarPointAt(msg.X, msg.Y, false)
		if !ok {
			m.clearSelection()
			return m, true
		}
		m.selection = selectionState{
			anchor:   point,
			focus:    point,
			active:   true,
			dragging: true,
			area:     selectionAreaSidebar,
		}
		return m, true

	case m.selection.dragging && m.selection.area == selectionAreaSidebar && msg.Action == tea.MouseActionMotion:
		point, ok := m.sidebarPointAt(msg.X, msg.Y, true)
		if !ok {
			return m, true
		}
		m.selection.focus = point
		return m, true

	case m.selection.dragging && m.selection.area == selectionAreaSidebar && msg.Action == tea.MouseActionRelease:
		point, ok := m.sidebarPointAt(msg.X, msg.Y, true)
		if ok {
			m.selection.focus = point
		}
		m.selection.dragging = false
		text := m.selectedText()
		if strings.TrimSpace(text) == "" {
			m.clearSelection()
			return m, true
		}
		if m.copySelection != nil {
			if err := m.copySelection(text); err != nil {
				m.status = "Copy failed: " + err.Error()
			} else {
				m.status = "Selection copied"
			}
		}
		return m, true
	}

	return m, false
}

func (m Model) handleInputSelectionMouse(msg tea.MouseMsg) (Model, bool) {
	if isWheelMouse(msg) {
		return m, false
	}

	switch {
	case msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress:
		point, ok := m.inputPointAt(msg.X, msg.Y, false)
		if !ok {
			return m, false
		}
		m.selection = selectionState{
			anchor:   point,
			focus:    point,
			active:   true,
			dragging: true,
			area:     selectionAreaInput,
		}
		return m, true

	case m.selection.dragging && m.selection.area == selectionAreaInput && msg.Action == tea.MouseActionMotion:
		point, ok := m.inputPointAt(msg.X, msg.Y, true)
		if !ok {
			return m, true
		}
		m.selection.focus = point
		return m, true

	case m.selection.dragging && m.selection.area == selectionAreaInput && msg.Action == tea.MouseActionRelease:
		point, ok := m.inputPointAt(msg.X, msg.Y, true)
		if ok {
			m.selection.focus = point
		}
		m.selection.dragging = false
		text := m.selectedText()
		if strings.TrimSpace(text) == "" {
			m.clearSelection()
			return m, true
		}
		if m.copySelection != nil {
			if err := m.copySelection(text); err != nil {
				m.status = "Copy failed: " + err.Error()
			} else {
				m.status = "Selection copied"
			}
		}
		return m, true
	}

	return m, false
}

func (m Model) transcriptPointAt(x int, y int, clamp bool) (selectionPoint, bool) {
	_, bodyHeight := m.contentDimensions()
	var layout transcriptLayout
	if m.cachedLayout != nil {
		layout = *m.cachedLayout
		visible := max(bodyHeight-bodyStyle.GetVerticalFrameSize(), 1)
		start := len(layout.styledLines) - visible - m.scrollOffset
		if start < 0 {
			start = 0
		}
		layout.visibleStart = start
		layout.visibleEnd = min(start+visible, len(layout.styledLines))
	} else {
		layout = m.transcriptLayout(m.chatWidth(), bodyHeight)
	}
	if len(layout.plainLines) == 0 || layout.visibleEnd <= layout.visibleStart {
		return selectionPoint{}, false
	}

	localX := x - viewPaddingLeft
	localY := y - viewPaddingTop
	if clamp {
		localX = max(0, min(localX, m.chatWidth()))
		localY = max(0, min(localY, layout.visibleEnd-layout.visibleStart-1))
	} else if localX < 0 || localX >= m.chatWidth() || localY < 0 || localY >= layout.visibleEnd-layout.visibleStart {
		return selectionPoint{}, false
	}

	line := layout.visibleStart + localY
	if line < 0 || line >= len(layout.plainLines) {
		return selectionPoint{}, false
	}
	col := max(0, min(localX, xansi.StringWidth(layout.plainLines[line])))
	return selectionPoint{line: line, col: col}, true
}

func (m Model) sidebarPointAt(x int, y int, clamp bool) (selectionPoint, bool) {
	if !m.shouldRenderSidebar() {
		return selectionPoint{}, false
	}
	_, contentHeight := m.contentDimensions()
	layout := m.sidebarLayout(m.sidebarWidth(), contentHeight)
	if len(layout.plainLines) == 0 {
		return selectionPoint{}, false
	}

	contentWidth, _ := m.contentDimensions()
	sidebarX := viewPaddingLeft + contentWidth + sidebarGap
	contentX := sidebarX + sidebarDividerWidth
	localX := x - contentX
	localY := y - viewPaddingTop
	if clamp {
		localX = max(0, min(localX, layout.width))
		localY = max(0, min(localY, len(layout.plainLines)-1))
	} else if localX < 0 || localX >= layout.width || localY < 0 || localY >= len(layout.plainLines) {
		return selectionPoint{}, false
	}

	col := max(0, min(localX, xansi.StringWidth(layout.plainLines[localY])))
	return selectionPoint{line: localY, col: col}, true
}

func (m Model) inputPointAt(x int, y int, clamp bool) (selectionPoint, bool) {
	rows := m.inputRenderRows()
	displayRows := m.inputDisplayRows(rows)
	if len(displayRows) == 0 {
		return selectionPoint{}, false
	}

	contentY := m.inputContentY()
	localY := y - contentY
	if clamp {
		localY = max(0, min(localY, len(displayRows)-1))
	} else if localY < 0 || localY >= len(displayRows) {
		return selectionPoint{}, false
	}

	displayRow := displayRows[localY]
	if displayRow.marker != "" {
		return selectionPoint{}, false
	}
	prefixWidth := xansi.StringWidth("  ")
	if localY == 0 {
		prefixWidth = xansi.StringWidth(inputPromptText(m))
	}
	localX := x - viewPaddingLeft - prefixWidth
	rowText := string(m.input[displayRow.row.start:displayRow.row.end])
	rowWidth := xansi.StringWidth(rowText)
	if clamp {
		localX = max(0, min(localX, rowWidth))
	} else if localX < 0 || localX > rowWidth {
		return selectionPoint{}, false
	}
	return selectionPoint{line: displayRow.index, col: localX}, true
}

func (m *Model) clearSelection() {
	m.selection = selectionState{}
}

func (m Model) selectedText() string {
	switch m.selection.area {
	case selectionAreaInput:
		return m.selectedInputText()
	case selectionAreaSidebar:
		return m.selectedSidebarText()
	}
	return m.selectedTranscriptText()
}

func (m Model) selectedTranscriptText() string {
	if !m.selection.active {
		return ""
	}
	layout := m.transcriptLayout(m.chatWidth(), m.transcriptHeight())
	return selectedTextFromLines(layout.plainLines, m.selection)
}

func (m Model) selectedSidebarText() string {
	if !m.selection.active {
		return ""
	}
	layout := m.sidebarLayout(m.sidebarWidth(), m.transcriptHeight())
	return selectedTextFromLines(layout.plainLines, m.selection)
}

func (m Model) selectedInputText() string {
	if !m.selection.active {
		return ""
	}
	rows := m.inputRenderRows()
	return selectedInputTextFromRows(m.input, rows, m.selection)
}

func (m Model) transcriptHeight() int {
	_, bodyHeight := m.contentDimensions()
	return bodyHeight
}

func selectedTextFromLines(lines []string, selection selectionState) string {
	if len(lines) == 0 {
		return ""
	}
	start, end := normalizedSelection(selection)
	if start.line < 0 {
		start.line = 0
	}
	if end.line < 0 {
		return ""
	}
	if start.line >= len(lines) {
		return ""
	}
	if end.line >= len(lines) {
		end.line = len(lines) - 1
		end.col = xansi.StringWidth(lines[end.line])
	}
	if start.line == end.line && start.col == end.col {
		return ""
	}

	out := make([]string, 0, end.line-start.line+1)
	for line := start.line; line <= end.line; line++ {
		plain := strings.TrimRight(lines[line], " \t")
		width := xansi.StringWidth(plain)
		left, right := 0, width
		if line == start.line {
			left = min(max(start.col, 0), width)
		}
		if line == end.line {
			right = min(max(end.col, 0), width)
		}
		if right < left {
			right = left
		}
		out = append(out, xansi.Cut(plain, left, right))
	}
	return strings.Join(out, "\n")
}

func selectedInputTextFromRows(input []rune, rows []inputRenderRow, selection selectionState) string {
	if len(rows) == 0 {
		return ""
	}
	start, end := normalizedSelection(selection)
	if start.line < 0 {
		start.line = 0
	}
	if end.line < 0 || start.line >= len(rows) {
		return ""
	}
	if end.line >= len(rows) {
		end.line = len(rows) - 1
		end.col = xansi.StringWidth(string(input[rows[end.line].start:rows[end.line].end]))
	}
	if start.line == end.line && start.col == end.col {
		return ""
	}

	out := make([]string, 0, end.line-start.line+1)
	for line := start.line; line <= end.line; line++ {
		row := rows[line]
		plain := string(input[row.start:row.end])
		width := xansi.StringWidth(plain)
		left, right := 0, width
		if line == start.line {
			left = min(max(start.col, 0), width)
		}
		if line == end.line {
			right = min(max(end.col, 0), width)
		}
		if right < left {
			right = left
		}
		out = append(out, xansi.Cut(plain, left, right))
	}
	return strings.Join(out, "\n")
}

func normalizedSelection(selection selectionState) (selectionPoint, selectionPoint) {
	start := selection.anchor
	end := selection.focus
	if end.line < start.line || (end.line == start.line && end.col < start.col) {
		start, end = end, start
	}
	return start, end
}

func copyToClipboard(text string) error {
	cmd := exec.Command("pbcopy")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_, writeErr := io.WriteString(stdin, text)
	closeErr := stdin.Close()
	waitErr := cmd.Wait()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return waitErr
}

func (m Model) sidebarIndexAt(x int, y int) (int, bool) {
	if !m.shouldRenderSidebar() {
		return 0, false
	}
	contentWidth, contentHeight := m.contentDimensions()
	sidebarX := viewPaddingLeft + contentWidth + sidebarGap
	sidebarHeight := sidebarBoxesHeight(contentHeight)
	width := m.sidebarWidth()
	sidebarY := viewPaddingTop
	localY := y - sidebarY
	if x < sidebarX || x >= sidebarX+width || localY < 0 || localY >= sidebarHeight {
		return 0, false
	}

	docs := m.sidebarDocuments
	if len(docs) == 0 {
		docs = loadSidebarDocuments(m.runner.WorkDir(), m.runner.SessionDir(), m.runner.AutoMemoryIndex())
	}
	heights := sidebarBoxHeights(sidebarDocumentAreaHeight(contentHeight, len(docs)), len(docs))
	top := 0
	for i, height := range heights {
		bottom := top + height
		if localY >= top && localY < bottom {
			return i, true
		}
		top = bottom
		if i < len(heights)-1 {
			separatorBottom := top + sidebarSeparatorHeight
			if localY >= top && localY < separatorBottom {
				return 0, false
			}
			top = separatorBottom
		}
	}
	return 0, false
}

func (m *Model) clampSidebarScrollOffsets() {
	docs := m.sidebarDocuments
	if len(docs) == 0 {
		docs = loadSidebarDocuments(m.runner.WorkDir(), m.runner.SessionDir(), m.runner.AutoMemoryIndex())
	}
	if !m.shouldRenderSidebar() || len(docs) == 0 {
		for i := range m.sidebarScrollOffsets {
			m.sidebarScrollOffsets[i] = 0
		}
		m.sidebarFocused = false
		return
	}

	_, contentHeight := m.contentDimensions()
	width := m.sidebarWidth()
	heights := sidebarBoxHeights(sidebarDocumentAreaHeight(contentHeight, len(docs)), len(docs))
	for i := range m.sidebarScrollOffsets {
		if i >= len(docs) || i >= len(heights) {
			m.sidebarScrollOffsets[i] = 0
			continue
		}
		maxOffset := maxSidebarScrollOffset(docs[i], sidebarContentWidth(width), heights[i])
		if m.sidebarScrollOffsets[i] < 0 {
			m.sidebarScrollOffsets[i] = 0
		}
		if m.sidebarScrollOffsets[i] > maxOffset {
			m.sidebarScrollOffsets[i] = maxOffset
		}
	}
	if !m.validSidebarIndex(m.sidebarFocusIndex) {
		m.sidebarFocusIndex = defaultSidebarFocusIndex(docs)
	}
}

func (m Model) shouldRenderSidebar() bool {
	return m.sidebarVisible && shouldRenderSidebar(m.width)
}

// handleAskDone replies to the engine with the overlay's collected result and
// clears the overlay, then re-arms the listener for the next question request.
func (m Model) handleAskDone() (tea.Model, tea.Cmd) {
	if m.askForm == nil {
		return m, nil
	}
	m.askForm.req.reply <- answerResult{
		answers:   m.askForm.answers,
		cancelled: m.askForm.cancelled,
	}
	m.askForm = nil
	if m.asker != nil {
		return m, listenForAsk(m.ctx, m.asker)
	}
	return m, nil
}

func (m Model) handlePlanReviewDone() (tea.Model, tea.Cmd) {
	if m.planForm == nil {
		return m, nil
	}
	result := planReviewResult{
		review:    m.planForm.review,
		cancelled: m.planForm.cancelled,
	}
	if result.review.Decision == tools.PlanApproved && !result.cancelled {
		m.collaborationMode = collaboration.ModeDefault
		m.runner.SetCollaborationMode(collaboration.ModeDefault)
		m.status = "Plan approved; continuing in Default mode"
	} else if result.cancelled {
		m.status = "Plan review cancelled; Formal Plan remains active"
	} else {
		m.status = "Continuing Formal Plan"
	}
	m.planForm.req.reply <- result
	m.planForm = nil
	if m.planReviewer != nil {
		return m, listenForPlanReview(m.ctx, m.planReviewer)
	}
	return m, nil
}

func (m Model) handleApprovalDone() (tea.Model, tea.Cmd) {
	if m.approvalForm == nil {
		return m, nil
	}
	m.approvalForm.req.reply <- m.approvalForm.decision()
	m.approvalForm = nil
	m.permissionSnapshot = permissionSnapshot(m.runner)
	if m.permissionBridge != nil {
		return m, listenForPermissionRequest(m.ctx, m.permissionBridge)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.approvalForm != nil {
		return m, m.approvalForm.update(msg)
	}
	if m.permissionForm != nil {
		return m, m.permissionForm.update(msg)
	}
	if m.effortForm != nil {
		return m, m.effortForm.update(msg)
	}
	if m.planForm != nil {
		return m, m.planForm.update(msg)
	}
	if m.askForm != nil {
		return m, m.askForm.update(msg)
	}

	if msg.Type == tea.KeyRunes {
		if next, cmd, ok := m.handleFragmentedMouseTail(msg.Runes); ok {
			return next, cmd
		}
	} else if len(m.mouseTail) > 0 {
		next, cmd := m.flushMouseTailAsInput()
		typed, ok := next.(Model)
		if !ok {
			return next, cmd
		}
		m = typed
		if cmd != nil {
			return m, cmd
		}
	}

	if !m.pendingEsc.IsZero() {
		m.pendingEsc = time.Time{}
		next, cmd := m.applyEscKey()
		typed, ok := next.(Model)
		if !ok {
			return next, cmd
		}
		m = typed
		if msg.Type == tea.KeyEsc {
			m, nextCmd := m.startPendingEsc()
			return m, tea.Batch(cmd, nextCmd)
		}
		next, nextCmd := m.handleKey(msg)
		return next, tea.Batch(cmd, nextCmd)
	}

	if m.rewindSelector != nil {
		next, cmd := m.rewindSelector.Update(msg)
		if typed, ok := next.(selector.Model); ok {
			m.rewindSelector = &typed
		}
		return m, cmd
	}

	key := msg.String()
	if key != "ctrl+c" {
		m.lastCtrlC = time.Time{}
	}
	if key != "esc" {
		m.lastEsc = time.Time{}
		m.lastEscAction = escActionNone
	}

	switch key {
	case "ctrl+c":
		if m.running && m.cancelRun != nil {
			m.cancelRun()
			m.status = "Cancel requested"
			m.appendEntry("system", "cancel", "Current run cancellation requested.", false)
			m.tryAutoRestoreAfterCancel()
			return m, nil
		}
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
	case "ctrl+f":
		return m.toggleSidebarFocus(), nil
	case "ctrl+o":
		m.toolOutputExpanded = !m.toolOutputExpanded
		m.cachedLayout = nil
		if m.toolOutputExpanded {
			m.status = "Tool output expanded"
		} else {
			m.status = "Tool output collapsed"
		}
		return m, nil
	}

	if m.sidebarFocused {
		return m.handleSidebarKey(msg)
	}

	switch key {
	case "esc":
		return m.startPendingEsc()
	case "enter":
		return m.submitInput()
	case "shift+enter", "ctrl+j":
		m.insertInputNewline()
		return m, nil
	case "shift+tab":
		return m.toggleFormalPlan()
	case "tab":
		if m.hasSlashMenu() {
			m.completeSlashCommand()
		} else if m.hasFileMentionMenu() {
			m.completeFileMention()
		}
		return m, nil
	case "backspace", "ctrl+h":
		if m.inputCursor > 0 {
			m.resetHistoryNavigation()
			m.clearInputPastePreview()
			m.input = append(m.input[:m.inputCursor-1], m.input[m.inputCursor:]...)
			m.inputCursor--
			m.updateCompletions()
		}
		return m, nil
	case "delete":
		if m.inputCursor < len(m.input) {
			m.resetHistoryNavigation()
			m.clearInputPastePreview()
			m.input = append(m.input[:m.inputCursor], m.input[m.inputCursor+1:]...)
			m.updateCompletions()
		}
		return m, nil
	case " ":
		m.resetHistoryNavigation()
		m.insertInputRunes([]rune{' '})
		m.updateCompletions()
		return m, nil
	case "ctrl+u":
		m.input = nil
		m.inputCursor = 0
		m.clearInputPastePreview()
		m.resetHistoryNavigation()
		m.resetCompletions()
		return m, nil
	case "left":
		m.moveInputCursorLeft()
		m.updateCompletions()
		return m, nil
	case "right":
		m.moveInputCursorRight()
		m.updateCompletions()
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
		if m.historyIndex == -1 && m.moveInputCursorUp() {
			m.updateCompletions()
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
		if m.historyIndex == -1 && m.moveInputCursorDown() {
			m.updateCompletions()
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
		if len(m.input) > 0 {
			m.moveInputCursorLineEnd()
			m.updateCompletions()
		} else {
			m.scrollOffset = 0
		}
		return m, nil
	case "home":
		m.moveInputCursorLineStart()
		m.updateCompletions()
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		pastedInput := msg.Paste || isMultilinePasteRunes(msg.Runes)
		runes := msg.Runes
		if pastedInput {
			runes = normalizeInputLineEndings(runes)
		}
		m.appendInputRunes(runes)
		if pastedInput {
			m.setInputPastePreview(runes)
		}
	}
	return m, nil
}

func (m *Model) insertInputNewline() {
	m.resetHistoryNavigation()
	m.insertInputRunes([]rune{'\n'})
	m.resetCompletions()
}

func (m Model) startPendingEsc() (tea.Model, tea.Cmd) {
	m.pendingEsc = m.nowTime()
	m.pendingEscID++
	return m, pendingEscCmd(m.pendingEscID)
}

func (m Model) applyEscKey() (tea.Model, tea.Cmd) {
	if m.running && m.cancelRun != nil {
		m.cancelRun()
		m.status = "Cancel requested"
		m.appendEntry("system", "cancel", "Current run cancellation requested.", false)
		return m, nil
	}
	now := m.nowTime()
	action := escActionRewind
	if len(m.input) > 0 {
		action = escActionClear
	}
	if !m.lastEsc.IsZero() && m.lastEscAction == action && now.Sub(m.lastEsc) <= quitConfirmWindow {
		m.lastEsc = time.Time{}
		m.lastEscAction = escActionNone
		if action == escActionClear {
			m.addInputHistory(string(m.input))
			m.input = nil
			m.inputCursor = 0
			m.clearInputPastePreview()
			m.resetHistoryNavigation()
			m.resetCompletions()
			m.status = "Input cleared"
			return m, nil
		}
		return m.openRewindSelector()
	}
	m.lastEsc = now
	m.lastEscAction = action
	if action == escActionClear {
		m.status = "Esc again to clear"
		return m, nil
	}
	m.status = "Press Esc again within 2s to rewind"
	return m, nil
}

func (m *Model) appendInputRunes(runes []rune) {
	m.insertInputRunes(runes)
}

func (m *Model) insertInputRunes(runes []rune) {
	if len(runes) == 0 {
		return
	}
	m.resetHistoryNavigation()
	m.clearInputPastePreview()
	m.clampInputCursor()
	next := make([]rune, 0, len(m.input)+len(runes))
	next = append(next, m.input[:m.inputCursor]...)
	next = append(next, runes...)
	next = append(next, m.input[m.inputCursor:]...)
	m.input = next
	m.inputCursor += len(runes)
	m.updateCompletions()
}

func isMultilinePasteRunes(runes []rune) bool {
	for _, r := range runes {
		if r == '\n' || r == '\r' {
			return true
		}
	}
	return false
}

func (m *Model) setInputPastePreview(pasted []rune) {
	lineCount := pastePreviewLineCount(pasted)
	if lineCount <= 1 {
		m.clearInputPastePreview()
		return
	}
	m.pastePreviewSeq++
	m.inputPastePreview = &inputPastePreview{
		id:        m.pastePreviewSeq,
		lineCount: lineCount,
	}
}

func (m *Model) clearInputPastePreview() {
	m.inputPastePreview = nil
}

func pastePreviewLineCount(pasted []rune) int {
	normalized := strings.ReplaceAll(string(pasted), "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	return strings.Count(normalized, "\n") + 1
}

func normalizeInputLineEndings(runes []rune) []rune {
	if len(runes) == 0 {
		return nil
	}
	out := make([]rune, 0, len(runes))
	for i := 0; i < len(runes); i++ {
		if runes[i] != '\r' {
			out = append(out, runes[i])
			continue
		}
		out = append(out, '\n')
		if i+1 < len(runes) && runes[i+1] == '\n' {
			i++
		}
	}
	return out
}

func (m Model) inputPastePreviewLabel() string {
	if m.inputPastePreview == nil {
		return ""
	}
	return fmt.Sprintf("[pasted text #%d +%d lines]", m.inputPastePreview.id, max(m.inputPastePreview.lineCount-1, 0))
}

func (m Model) handleFragmentedMouseTail(runes []rune) (tea.Model, tea.Cmd, bool) {
	if len(runes) == 0 {
		return m, nil, false
	}
	if len(m.mouseTail) > 0 {
		return m.consumeMouseTail(runes, true)
	}
	if tails, partial, ok := parseMouseTailRunes(runes); ok {
		hadEsc := !m.pendingEsc.IsZero()
		m.pendingEsc = time.Time{}
		if len(tails) > 0 {
			m.applyMouseTails(tails)
		}
		if len(partial) > 0 {
			cmd := m.startMouseTail(partial, hadEsc)
			return m, cmd, true
		}
		return m, nil, true
	}
	if !m.pendingEsc.IsZero() && isMouseTailPrefix(runes) {
		cmd := m.startMouseTail(runes, true)
		m.pendingEsc = time.Time{}
		return m, cmd, true
	}
	if isMouseTailPrefix(runes) {
		cmd := m.startMouseTail(runes, false)
		return m, cmd, true
	}
	return m, nil, false
}

func (m *Model) startMouseTail(runes []rune, hadEsc bool) tea.Cmd {
	m.mouseTail = append([]rune(nil), runes...)
	m.mouseTailEsc = hadEsc
	m.mouseTailID++
	return mouseTailCmd(m.mouseTailID)
}

func (m Model) consumeMouseTail(runes []rune, applyEscOnInvalid bool) (tea.Model, tea.Cmd, bool) {
	combined := make([]rune, 0, len(m.mouseTail)+len(runes))
	combined = append(combined, m.mouseTail...)
	combined = append(combined, runes...)
	tails, partial, ok := parseMouseTailRunes(combined)
	if ok {
		m.mouseTail = nil
		m.applyMouseTails(tails)
		if len(partial) > 0 {
			cmd := m.startMouseTail(partial, m.mouseTailEsc)
			return m, cmd, true
		} else {
			m.mouseTailEsc = false
		}
		return m, nil, true
	}
	if !applyEscOnInvalid {
		return m, nil, false
	}
	hadEsc := m.mouseTailEsc
	m.mouseTail = nil
	m.mouseTailEsc = false
	var cmd tea.Cmd
	if hadEsc {
		next, nextCmd := m.applyEscKey()
		typed, ok := next.(Model)
		if !ok {
			return next, nextCmd, true
		}
		m = typed
		cmd = nextCmd
	}
	m.appendInputRunes(combined)
	return m, cmd, true
}

func (m Model) flushMouseTailAsInput() (tea.Model, tea.Cmd) {
	if len(m.mouseTail) == 0 {
		return m, nil
	}
	payload := append([]rune(nil), m.mouseTail...)
	hadEsc := m.mouseTailEsc
	m.mouseTail = nil
	m.mouseTailEsc = false
	var cmd tea.Cmd
	if hadEsc {
		next, nextCmd := m.applyEscKey()
		typed, ok := next.(Model)
		if !ok {
			return next, nextCmd
		}
		m = typed
		cmd = nextCmd
	}
	m.appendInputRunes(payload)
	return m, cmd
}

func (m *Model) applyMouseTails(tails []mouseTailEvent) {
	for _, tail := range tails {
		button := tea.MouseButton(0)
		switch {
		case (tail.button & 0x43) == 0x40:
			button = tea.MouseButtonWheelUp
		case (tail.button & 0x43) == 0x41:
			button = tea.MouseButtonWheelDown
		default:
			continue
		}
		next, _ := m.handleMouse(tea.MouseMsg{X: tail.col - 1, Y: tail.row - 1, Button: button})
		if typed, ok := next.(Model); ok {
			*m = typed
		}
	}
}

type mouseTailEvent struct {
	button int
	col    int
	row    int
}

func parseMouseTailRunes(runes []rune) ([]mouseTailEvent, []rune, bool) {
	var tails []mouseTailEvent
	for len(runes) > 0 {
		tail, consumed, partial, ok := parseOneMouseTail(runes)
		if !ok {
			return nil, nil, false
		}
		if partial {
			return tails, append([]rune(nil), runes...), true
		}
		tails = append(tails, tail)
		runes = runes[consumed:]
	}
	return tails, nil, len(tails) > 0
}

func isMouseTailPrefix(runes []rune) bool {
	_, _, partial, ok := parseOneMouseTail(runes)
	return ok && partial
}

func parseOneMouseTail(runes []rune) (mouseTailEvent, int, bool, bool) {
	var tail mouseTailEvent
	if len(runes) == 0 {
		return tail, 0, true, true
	}
	if runes[0] != '[' {
		return tail, 0, false, false
	}
	if len(runes) == 1 {
		return tail, 0, true, true
	}
	if runes[1] != '<' && runes[1] != '>' {
		return tail, 0, false, false
	}
	i := 2
	fields := [3]int{}
	for field := 0; field < 3; field++ {
		if i >= len(runes) {
			return tail, 0, true, true
		}
		start := i
		value := 0
		for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
			value = value*10 + int(runes[i]-'0')
			i++
		}
		if i == start {
			return tail, 0, false, false
		}
		fields[field] = value
		if field < 2 {
			if i >= len(runes) {
				return tail, 0, true, true
			}
			if runes[i] != ';' {
				return tail, 0, false, false
			}
			i++
		}
	}
	if i >= len(runes) {
		return tail, 0, true, true
	}
	if runes[i] != 'M' && runes[i] != 'm' {
		return tail, 0, false, false
	}
	tail = mouseTailEvent{button: fields[0], col: fields[1], row: fields[2]}
	return tail, i + 1, false, true
}

func (m Model) handleSidebarKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.sidebarFocused = false
		m.status = "Sidebar focus off"
	case "tab":
		m.moveSidebarFocus(1)
	case "shift+tab":
		m.moveSidebarFocus(-1)
	case "1", "2", "3":
		m.selectSidebarIndex(int(msg.String()[0] - '1'))
	case "up":
		m.scrollFocusedSidebar(-1)
	case "down":
		m.scrollFocusedSidebar(1)
	case "pgup":
		m.scrollFocusedSidebar(-scrollDelta(msg.String()))
	case "pgdown":
		m.scrollFocusedSidebar(scrollDelta(msg.String()))
	case "home":
		m.setFocusedSidebarOffset(0)
	case "end":
		m.setFocusedSidebarOffset(m.maxFocusedSidebarOffset())
	}
	return m, nil
}

func (m Model) toggleSidebarFocus() Model {
	if m.sidebarFocused {
		m.sidebarFocused = false
		m.status = "Sidebar focus off"
		return m
	}
	if !m.shouldRenderSidebar() || len(m.sidebarDocuments) == 0 {
		m.status = "Sidebar unavailable"
		return m
	}
	m.sidebarFocused = true
	if !m.validSidebarIndex(m.sidebarFocusIndex) {
		m.sidebarFocusIndex = defaultSidebarFocusIndex(m.sidebarDocuments)
	}
	m.status = fmt.Sprintf("Sidebar focus: %s", m.sidebarDocuments[m.sidebarFocusIndex].Title)
	return m
}

func (m *Model) moveSidebarFocus(delta int) {
	if len(m.sidebarDocuments) == 0 {
		m.sidebarFocused = false
		return
	}
	next := m.sidebarFocusIndex + delta
	for next < 0 {
		next += len(m.sidebarDocuments)
	}
	next %= len(m.sidebarDocuments)
	m.selectSidebarIndex(next)
}

func (m *Model) selectSidebarIndex(index int) {
	if !m.validSidebarIndex(index) {
		return
	}
	m.sidebarFocusIndex = index
	m.status = fmt.Sprintf("Sidebar focus: %s", m.sidebarDocuments[index].Title)
}

func (m Model) validSidebarIndex(index int) bool {
	return index >= 0 && index < len(m.sidebarDocuments) && index < len(m.sidebarScrollOffsets)
}

func defaultSidebarFocusIndex(docs []sidebarDocument) int {
	for i, doc := range docs {
		if doc.Title == "Plan" {
			return i
		}
	}
	return 0
}

func (m *Model) scrollFocusedSidebar(delta int) {
	if !m.validSidebarIndex(m.sidebarFocusIndex) {
		return
	}
	m.setFocusedSidebarOffset(m.sidebarScrollOffsets[m.sidebarFocusIndex] + delta)
}

func (m *Model) setFocusedSidebarOffset(offset int) {
	if !m.validSidebarIndex(m.sidebarFocusIndex) {
		return
	}
	maxOffset := m.maxFocusedSidebarOffset()
	if offset < 0 {
		offset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	m.sidebarScrollOffsets[m.sidebarFocusIndex] = offset
}

func (m Model) maxFocusedSidebarOffset() int {
	if !m.validSidebarIndex(m.sidebarFocusIndex) {
		return 0
	}
	_, contentHeight := m.contentDimensions()
	width := m.sidebarWidth()
	heights := sidebarBoxHeights(sidebarDocumentAreaHeight(contentHeight, len(m.sidebarDocuments)), len(m.sidebarDocuments))
	if m.sidebarFocusIndex >= len(heights) {
		return 0
	}
	return maxSidebarScrollOffset(m.sidebarDocuments[m.sidebarFocusIndex], sidebarContentWidth(width), heights[m.sidebarFocusIndex])
}

func (m Model) toggleFormalPlan() (tea.Model, tea.Cmd) {
	mode := collaboration.ModeFormalPlan
	if m.collaborationMode == collaboration.ModeFormalPlan {
		mode = collaboration.ModeDefault
	}
	return m.selectCollaborationMode(mode)
}

func (m Model) selectCollaborationMode(mode collaboration.Mode) (tea.Model, tea.Cmd) {
	m.collaborationMode = collaboration.Normalize(mode)
	m.runner.SetCollaborationMode(m.collaborationMode)
	if m.collaborationMode == collaboration.ModeFormalPlan {
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
	m.clearInputPastePreview()
	completed := command.Name
	if command.hasArgumentHint() {
		completed += " "
	}
	m.input = []rune(completed)
	m.inputCursor = len(m.input)
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
	wasAtEnd := end == len(m.input)
	next := make([]rune, 0, len(m.input)-end+start+len(replacement)+1)
	next = append(next, m.input[:start]...)
	next = append(next, replacement...)
	if wasAtEnd {
		next = append(next, ' ')
	} else {
		next = append(next, m.input[end:]...)
	}
	m.clearInputPastePreview()
	m.input = next
	m.inputCursor = start + len(replacement)
	if wasAtEnd {
		m.inputCursor++
	}
	m.updateCompletions()
}

func (m Model) submitInput() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(string(m.input))
	if text == "" {
		return m, nil
	}
	if strings.HasPrefix(text, "!") {
		return m.submitBangCommand(text)
	}
	if strings.HasPrefix(text, "/") {
		if command, ok := m.selectedSlashCommand(); ok {
			text = command.Name
		}
		m.addInputHistory(text)
		m.input = nil
		m.inputCursor = 0
		m.clearInputPastePreview()
		m.resetHistoryNavigation()
		m.resetCompletions()
		if m.running && isQueuedModelCommand(text) {
			m.addInputHistory(text)
			m.queuePrompt(text)
			m.status = fmt.Sprintf("Queued %d message%s", len(m.queuedPrompts), pluralS(len(m.queuedPrompts)))
			return m, nil
		}
		return m.handleSlashCommand(text)
	}
	if m.running {
		m.addInputHistory(text)
		m.input = nil
		m.inputCursor = 0
		m.clearInputPastePreview()
		m.resetHistoryNavigation()
		m.resetCompletions()
		m.queuePrompt(text)
		m.status = fmt.Sprintf("Queued %d message%s", len(m.queuedPrompts), pluralS(len(m.queuedPrompts)))
		return m, nil
	}

	m.addInputHistory(text)
	m.input = nil
	m.inputCursor = 0
	m.clearInputPastePreview()
	m.resetHistoryNavigation()
	m.resetCompletions()
	return m.startPrompt(text)
}

func (m Model) submitBangCommand(text string) (tea.Model, tea.Cmd) {
	command := strings.TrimSpace(strings.TrimPrefix(text, "!"))
	if command == "" {
		m.input = nil
		m.inputCursor = 0
		m.clearInputPastePreview()
		m.resetHistoryNavigation()
		m.resetCompletions()
		m.appendCommandEntry("Shell commands", "Prefix a command with ! to run it locally.\nExample: !ls")
		m.status = "Shell command help"
		return m, nil
	}
	if m.running {
		m.status = "Shell command unavailable while a run is active"
		return m, nil
	}

	m.addInputHistory("!" + command)
	m.input = nil
	m.inputCursor = 0
	m.clearInputPastePreview()
	m.resetHistoryNavigation()
	m.resetCompletions()
	m.scrollOffset = 0
	m.running = true
	m.runStartedAt = m.nowTime()
	m.spinnerFrame = 0
	m.status = "Running shell command"

	runCtx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel
	return m, runShellCommandCmd(runCtx, m.runner.WorkDir(), command)
}

func (m Model) startPrompt(text string) (tea.Model, tea.Cmd) {
	return m.startPromptInCollaborationMode(text, m.collaborationMode)
}

func (m Model) startPromptInCollaborationMode(text string, mode collaboration.Mode) (tea.Model, tea.Cmd) {
	m.scrollOffset = 0
	m.running = true
	m.runStartedAt = m.nowTime()
	m.spinnerFrame = 0
	m.status = "Running"
	m.appendEntry("user", "you", text, false)

	runCtx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel
	return m, runPromptCmd(runCtx, m.runner, text, mode, m.events)
}

func (m Model) startNextQueuedPrompt() (tea.Model, tea.Cmd) {
	for len(m.queuedPrompts) > 0 {
		queued := m.queuedPrompts[0]
		m.queuedPrompts = append([]queuedPrompt(nil), m.queuedPrompts[1:]...)
		if isModelCommand(queued.text) {
			next, cmd := m.handleSlashCommand(queued.text)
			m = next.(Model)
			if cmd != nil {
				return m, cmd
			}
			continue
		}
		next, cmd := m.startPromptInCollaborationMode(queued.text, queued.mode)
		typed := next.(Model)
		if len(typed.queuedPrompts) > 0 {
			typed.status = fmt.Sprintf("Running queued message; %d queued", len(typed.queuedPrompts))
		} else {
			typed.status = "Running queued message"
		}
		return typed, cmd
	}
	return m, nil
}

func (m *Model) queuePrompt(text string) {
	m.queuedPrompts = append(m.queuedPrompts, queuedPrompt{
		text: text,
		mode: collaboration.Normalize(m.collaborationMode),
	})
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

func (m *Model) clampInputCursor() {
	if m.inputCursor < 0 {
		m.inputCursor = 0
	}
	if m.inputCursor > len(m.input) {
		m.inputCursor = len(m.input)
	}
}

func (m *Model) moveInputCursorLeft() {
	m.clampInputCursor()
	if m.inputCursor > 0 {
		m.inputCursor--
	}
}

func (m *Model) moveInputCursorRight() {
	m.clampInputCursor()
	if m.inputCursor < len(m.input) {
		m.inputCursor++
	}
}

func (m *Model) moveInputCursorUp() bool {
	return m.moveInputCursorVertical(-1)
}

func (m *Model) moveInputCursorDown() bool {
	return m.moveInputCursorVertical(1)
}

func (m *Model) moveInputCursorVertical(delta int) bool {
	m.clampInputCursor()
	points := m.inputCursorPoints()
	if len(points) == 0 {
		return false
	}
	current := points[m.inputCursor]
	targetRow := current.row + delta
	if targetRow < 0 {
		if m.inputCursor > 0 {
			m.captureHistoryDraftCursor()
			m.inputCursor = 0
			return true
		}
		return false
	}
	lastRow := points[len(points)-1].row
	if targetRow > lastRow {
		if m.inputCursor < len(m.input) {
			m.captureHistoryDraftCursor()
			m.inputCursor = len(m.input)
			return true
		}
		return false
	}
	next := closestInputCursorOnRow(points, targetRow, current.col)
	if next == m.inputCursor {
		return false
	}
	m.inputCursor = next
	return true
}

func (m *Model) moveInputCursorLineStart() {
	m.clampInputCursor()
	points := m.inputCursorPoints()
	if len(points) == 0 {
		return
	}
	row := points[m.inputCursor].row
	for i, point := range points {
		if point.row == row {
			m.inputCursor = i
			return
		}
	}
}

func (m *Model) moveInputCursorLineEnd() {
	m.clampInputCursor()
	points := m.inputCursorPoints()
	if len(points) == 0 {
		return
	}
	row := points[m.inputCursor].row
	end := m.inputCursor
	for i, point := range points {
		if point.row == row {
			end = i
		}
	}
	m.inputCursor = end
}

type inputCursorPoint struct {
	row int
	col int
}

type inputRenderRow struct {
	start int
	end   int
}

func (m Model) inputCursorPoints() []inputCursorPoint {
	rows := m.inputRenderRows()
	points := make([]inputCursorPoint, len(m.input)+1)
	for rowIndex, row := range rows {
		col := 0
		for i := row.start; i <= row.end; i++ {
			if i >= len(points) {
				break
			}
			points[i] = inputCursorPoint{row: rowIndex, col: col}
			if i < row.end {
				col += inputRuneWidth(m.input[i])
			}
		}
		if row.end < len(m.input) && m.input[row.end] == '\n' {
			points[row.end] = inputCursorPoint{row: rowIndex, col: col}
		}
	}
	return points
}

func (m Model) inputRenderRows() []inputRenderRow {
	width := m.inputTextWidth()
	rows := make([]inputRenderRow, 0, 1)
	start, col := 0, 0
	for i, r := range m.input {
		if r == '\n' {
			rows = append(rows, inputRenderRow{start: start, end: i})
			start = i + 1
			col = 0
			continue
		}
		runeWidth := inputRuneWidth(r)
		if col > 0 && col+runeWidth > width {
			rows = append(rows, inputRenderRow{start: start, end: i})
			start = i
			col = 0
		}
		col += runeWidth
		if col >= width {
			rows = append(rows, inputRenderRow{start: start, end: i + 1})
			start = i + 1
			col = 0
		}
	}
	rows = append(rows, inputRenderRow{start: start, end: len(m.input)})
	return rows
}

func inputRuneWidth(r rune) int {
	width := xansi.StringWidth(string(r))
	if width < 1 {
		return 1
	}
	return width
}

func (m Model) inputTextWidth() int {
	width := m.innerWidth() - inputStyle.GetHorizontalFrameSize() - xansi.StringWidth("> ") - 1
	if width < 1 {
		return 1
	}
	return width
}

func closestInputCursorOnRow(points []inputCursorPoint, row int, col int) int {
	best := -1
	bestDistance := 0
	for i, point := range points {
		if point.row != row {
			continue
		}
		distance := point.col - col
		if distance < 0 {
			distance = -distance
		}
		if best == -1 || distance < bestDistance || (distance == bestDistance && point.col <= col) {
			best = i
			bestDistance = distance
		}
	}
	if best == -1 {
		return 0
	}
	return best
}

func (m *Model) recallPreviousInput() {
	if len(m.inputHistory) == 0 {
		return
	}
	if m.historyIndex == -1 {
		m.captureHistoryDraftCursor()
		m.historyIndex = len(m.inputHistory) - 1
	} else if m.historyIndex > 0 {
		m.historyIndex--
	}
	m.input = []rune(m.inputHistory[m.historyIndex])
	m.inputCursor = len(m.input)
	m.clearInputPastePreview()
}

func (m *Model) recallNextInput() {
	if m.historyIndex == -1 {
		return
	}
	if m.historyIndex < len(m.inputHistory)-1 {
		m.historyIndex++
		m.input = []rune(m.inputHistory[m.historyIndex])
		m.inputCursor = len(m.input)
		m.clearInputPastePreview()
		return
	}
	m.historyIndex = -1
	m.input = append([]rune(nil), m.historyDraft...)
	m.inputCursor = m.historyDraftCursor
	m.clampInputCursor()
	m.historyDraft = nil
	m.historyDraftCursor = 0
	m.clearInputPastePreview()
}

func (m *Model) resetHistoryNavigation() {
	m.historyIndex = -1
	m.historyDraft = nil
	m.historyDraftCursor = 0
}

func (m *Model) captureHistoryDraftCursor() {
	if m.historyDraft != nil {
		return
	}
	m.historyDraft = append([]rune(nil), m.input...)
	m.historyDraftCursor = m.inputCursor
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

// hasBangPrefix reports whether the current input starts with the bang shell
// prefix, so the prompt can switch to shell mode while the user is still typing.
func (m Model) hasBangPrefix() bool {
	return len(m.input) > 0 && m.input[0] == '!'
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
	case "/status", "/session":
		m.permissionSnapshot = permissionSnapshot(m.runner)
		m.appendCommandEntry("Status", m.formatStatusOverview())
		m.status = "Status"
		return m, nil
	case "/rewind", "/checkpoint":
		return m.openRewindSelector()
	case "/model":
		if len(fields) == 1 {
			m.appendCommandEntry("Model", fmt.Sprintf("Current model: %s\nUsage: /model <model_name>", m.runner.Model()))
			m.status = "Model"
			return m, nil
		}
		if len(fields) > 2 {
			m.appendEntry("error", "invalid command", "Usage: /model <model_name>", true)
			m.status = "Invalid model command"
			return m, nil
		}
		modelName := strings.TrimSpace(fields[1])
		if err := m.runner.SetModel(modelName); err != nil {
			m.appendEntry("error", "model switch failed", err.Error(), true)
			m.status = "Model switch failed"
			return m, nil
		}
		m.modelName = m.runner.Model()
		m.refreshRuntimeInfo()
		m.appendCommandEntry("Model", fmt.Sprintf("Switched model to %s", m.modelName))
		m.status = fmt.Sprintf("Model switched to %s", m.modelName)
		return m, nil
	case "/plan":
		if len(fields) == 1 {
			return m.selectCollaborationMode(collaboration.ModeFormalPlan)
		}
		if len(fields) == 2 && strings.EqualFold(fields[1], "off") {
			return m.selectCollaborationMode(collaboration.ModeDefault)
		}
		m.appendEntry("error", "invalid command", "Usage: /plan [off]", true)
		m.status = "Invalid plan command"
		return m, nil
	case "/theme":
		return m.handleThemeCommand(fields)
	case "/statusline":
		return m.handleStatuslineCommand(text, fields)
	case "/permissions":
		return m.handlePermissionsCommand(fields)
	case "/effort":
		return m.handleEffortCommand(fields)
	case "/sidebar":
		mode := "toggle"
		if len(fields) > 1 {
			mode = strings.ToLower(fields[1])
		}
		switch mode {
		case "on", "show", "true", "1":
			m.sidebarVisible = true
			m.status = "Sidebar shown"
		case "off", "hide", "false", "0":
			m.sidebarVisible = false
			m.sidebarFocused = false
			m.status = "Sidebar hidden"
		case "toggle":
			m.sidebarVisible = !m.sidebarVisible
			if m.sidebarVisible {
				m.status = "Sidebar shown"
			} else {
				m.sidebarFocused = false
				m.status = "Sidebar hidden"
			}
		default:
			m.appendEntry("error", "invalid command", "Usage: /sidebar [on|off|toggle]", true)
			m.status = "Invalid sidebar command"
			return m, nil
		}
		m.clampSidebarScrollOffsets()
		return m, nil
	case "/compact":
		if m.running {
			m.status = "Cannot compact while a run is active"
			return m, nil
		}
		customInstructions := ""
		if len(fields) > 1 {
			customInstructions = strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
		}
		m.running = true
		m.runStartedAt = m.nowTime()
		m.spinnerFrame = 0
		m.status = "Compacting context"
		return m, compactNowCmd(m.ctx, m.runner, customInstructions)
	case "/clear", "/new":
		if m.running {
			m.status = "Cannot create a new session while a run is active"
			return m, nil
		}
		m.running = true
		m.runStartedAt = m.nowTime()
		m.spinnerFrame = 0
		m.status = "Creating new session"
		return m, newSessionCmd(m.ctx, m.runner)
	case "/autodev":
		backlogPath := ""
		if len(fields) > 1 {
			backlogPath = strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
		}
		return m.handleAutodevCommand(backlogPath)
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
		if pc, args, ok := m.lookupPromptCommand(text); ok {
			return m.executePromptCommand(pc, args, text)
		}
		m.appendEntry("error", "unknown command", fmt.Sprintf("Unknown command: %s", cmd), true)
		m.status = "Unknown command"
		return m, nil
	}
}

func (m Model) handleEffortCommand(fields []string) (tea.Model, tea.Cmd) {
	if len(fields) != 1 {
		m.appendEntry("error", "invalid command", "Usage: /effort", true)
		m.status = "Invalid effort command"
		return m, nil
	}
	options, err := effort.OptionsForProtocol(m.providerProtocol)
	if err != nil {
		m.appendEntry("error", "effort unavailable", err.Error(), true)
		m.status = "Effort unavailable"
		return m, nil
	}
	selected := m.effortValue
	if selected == "" {
		selected = effort.Auto
	}
	m.effortForm = newEffortForm(m.providerProtocol, options, selected)
	m.status = "Effort"
	return m, nil
}

func (m Model) handleThemeCommand(fields []string) (tea.Model, tea.Cmd) {
	if len(fields) == 1 {
		m.appendCommandEntry("Theme", fmt.Sprintf(
			"Current: %s\nAvailable: %s\nUsage: /theme <name>",
			m.themeName,
			strings.Join(builtInThemeNames(), ", "),
		))
		m.status = "Theme"
		return m, nil
	}
	if len(fields) != 2 {
		m.appendEntry("error", "invalid command", "Usage: /theme <name>", true)
		m.status = "Invalid theme command"
		return m, nil
	}
	nextTheme := normalizeThemeName(fields[1])
	if !isBuiltInTheme(nextTheme) {
		m.appendEntry("error", "unknown theme", fmt.Sprintf("Unknown theme: %s\nAvailable: %s", fields[1], strings.Join(builtInThemeNames(), ", ")), true)
		m.status = "Unknown theme"
		return m, nil
	}
	previousTheme := m.themeName
	m.themeName = applyTheme(nextTheme)
	if err := m.saveTUISettings(); err != nil {
		m.themeName = applyTheme(previousTheme)
		m.appendEntry("error", "theme save failed", fmt.Sprintf("save settings: %v", err), true)
		m.status = "Theme save failed"
		return m, nil
	}
	m.appendCommandEntry("Theme", fmt.Sprintf("Theme set to %s", m.themeName))
	m.status = fmt.Sprintf("Theme set to %s", m.themeName)
	return m, nil
}

func (m Model) handleStatuslineCommand(text string, fields []string) (tea.Model, tea.Cmd) {
	if len(fields) == 1 {
		m.appendCommandEntry("Statusline", m.formatStatuslineHelp())
		m.status = "Statusline"
		return m, nil
	}
	subcommand := strings.ToLower(fields[1])
	switch subcommand {
	case "default":
		if len(fields) != 2 {
			m.appendEntry("error", "invalid command", "Usage: /statusline default", true)
			m.status = "Invalid statusline command"
			return m, nil
		}
		previousItems := append([]string(nil), m.statuslineItems...)
		m.statuslineItems = append([]string(nil), defaultStatuslineItems...)
		if err := m.saveTUISettings(); err != nil {
			m.statuslineItems = previousItems
			m.appendEntry("error", "statusline save failed", fmt.Sprintf("save settings: %v", err), true)
			m.status = "Statusline save failed"
			return m, nil
		}
		m.appendCommandEntry("Statusline", "Statusline reset to default: "+statuslineItemsText(m.statuslineItems))
		m.status = "Statusline reset"
		return m, nil
	case "set":
		rawItems := commandArgsAfter(text, fields[0], fields[1])
		items, err := parseStatuslineItems(rawItems)
		if err != nil {
			m.appendEntry("error", "invalid statusline", err.Error(), true)
			m.status = "Invalid statusline"
			return m, nil
		}
		previousItems := append([]string(nil), m.statuslineItems...)
		m.statuslineItems = items
		if err := m.saveTUISettings(); err != nil {
			m.statuslineItems = previousItems
			m.appendEntry("error", "statusline save failed", fmt.Sprintf("save settings: %v", err), true)
			m.status = "Statusline save failed"
			return m, nil
		}
		m.appendCommandEntry("Statusline", "Statusline set to: "+statuslineItemsText(m.statuslineItems))
		m.status = "Statusline updated"
		return m, nil
	default:
		m.appendEntry("error", "invalid command", "Usage: /statusline [set <items>|default]", true)
		m.status = "Invalid statusline command"
		return m, nil
	}
}

func (m Model) formatStatuslineHelp() string {
	return fmt.Sprintf(
		"Current: %s\nDefault: %s\nAvailable: %s\nUsage: /statusline set <items>\nUsage: /statusline default",
		statuslineItemsText(m.statuslineItems),
		statuslineItemsText(defaultStatuslineItems),
		statuslineItemsText(statuslineAvailableItems()),
	)
}

func (m Model) handlePermissionsCommand(fields []string) (tea.Model, tea.Cmd) {
	if len(fields) == 1 {
		m.permissionSnapshot = permissionSnapshot(m.runner)
		m.permissionForm = newPermissionForm(m.permissionSnapshot)
		m.status = "Permissions"
		return m, nil
	}
	switch strings.ToLower(fields[1]) {
	case "ask":
		return m.setPermissionMode(permission.ModeAsk, false, false)
	case "approve":
		return m.setPermissionMode(permission.ModeApprove, false, false)
	case "full-access", "full_access":
		confirm := len(fields) > 2 && (fields[2] == "confirm" || fields[2] == "--confirm")
		remember := len(fields) > 2 && (fields[2] == "remember" || fields[2] == "--remember")
		if !confirm && !remember {
			m.permissionSnapshot = permissionSnapshot(m.runner)
			m.permissionForm = newFullAccessWarningForm(m.permissionSnapshot)
			m.status = "Full Access warning"
			return m, nil
		}
		return m.setPermissionMode(permission.ModeFullAccess, remember, confirm)
	case "clear":
		count := clearPermissionGrants(m.runner)
		m.permissionSnapshot = permissionSnapshot(m.runner)
		m.appendCommandEntry("Permissions", fmt.Sprintf("Cleared %d session approval(s).", count))
		m.status = "Session approvals cleared"
		return m, nil
	default:
		m.appendEntry("error", "invalid command", "Usage: /permissions [ask|approve|full-access [confirm|remember]|clear]", true)
		m.status = "Invalid permissions command"
		return m, nil
	}
}

func (m Model) handlePermissionDone() (tea.Model, tea.Cmd) {
	if m.permissionForm == nil {
		return m, nil
	}
	result := m.permissionForm.result
	m.permissionForm = nil
	switch result {
	case permissionFormAsk:
		return m.setPermissionMode(permission.ModeAsk, false, false)
	case permissionFormApprove:
		return m.setPermissionMode(permission.ModeApprove, false, false)
	case permissionFormFullAccessSession:
		return m.setPermissionMode(permission.ModeFullAccess, false, true)
	case permissionFormFullAccessRemember:
		return m.setPermissionMode(permission.ModeFullAccess, true, true)
	case permissionFormClear:
		count := clearPermissionGrants(m.runner)
		m.permissionSnapshot = permissionSnapshot(m.runner)
		m.appendCommandEntry("Permissions", fmt.Sprintf("Cleared %d session approval(s).", count))
		m.status = "Session approvals cleared"
		return m, nil
	default:
		m.status = "Permissions cancelled"
		return m, nil
	}
}

func (m Model) handleEffortDone() (tea.Model, tea.Cmd) {
	if m.effortForm == nil {
		return m, nil
	}
	value := strings.TrimSpace(m.effortForm.result)
	m.effortForm = nil
	if value == "" {
		m.status = "Effort cancelled"
		return m, nil
	}
	if err := m.saveEffortSetting(value); err != nil {
		m.appendEntry("error", "effort save failed", fmt.Sprintf("save settings: %v", err), true)
		m.status = "Effort save failed"
		return m, nil
	}
	m.effortValue = value
	if runtime, ok := m.runner.(effortRuntime); ok {
		if value == effort.Auto {
			runtime.SetEffortOverride("")
		} else {
			runtime.SetEffortOverride(value)
		}
	}
	m.appendCommandEntry("Effort", fmt.Sprintf("Effort set to %s for %s", value, m.providerProtocol))
	m.status = fmt.Sprintf("Effort set to %s", value)
	return m, nil
}

func (m Model) setPermissionMode(mode permission.Mode, remember bool, confirm bool) (tea.Model, tea.Cmd) {
	previous := permissionSnapshot(m.runner)
	nextSettings := settings.PermissionSettings{
		Mode:                        string(mode),
		FullAccessWarningRemembered: previous.FullAccessRemembered,
	}
	if mode == permission.ModeFullAccess && remember {
		nextSettings.FullAccessWarningRemembered = true
	}
	if err := m.savePermissionSettings(nextSettings); err != nil {
		m.appendEntry("error", "permissions save failed", fmt.Sprintf("save settings: %v", err), true)
		m.status = "Permissions save failed"
		return m, nil
	}
	if mode == permission.ModeFullAccess {
		setPermissionMode(m.runner, permission.ModeFullAccess, nextSettings.FullAccessWarningRemembered)
		if remember || confirm {
			activateFullAccess(m.runner, remember)
		}
	} else {
		setPermissionMode(m.runner, mode, nextSettings.FullAccessWarningRemembered)
	}
	m.permissionSnapshot = permissionSnapshot(m.runner)
	m.appendCommandEntry("Permissions", m.formatPermissionsHelp())
	m.status = "Permissions updated"
	return m, nil
}

func (m Model) formatPermissionsHelp() string {
	snap := m.permissionSnapshot
	return fmt.Sprintf(
		"Selected: %s\nEffective: %s\nSession approvals: %d\nFull Access warning remembered: %s\n\nUsage: /permissions ask\nUsage: /permissions approve\nUsage: /permissions full-access\nUsage: /permissions full-access confirm\nUsage: /permissions full-access remember\nUsage: /permissions clear",
		permissionModeLabel(snap.SelectedMode),
		permissionModeLabel(snap.EffectiveMode),
		snap.SessionGrantCount,
		onOff(snap.FullAccessRemembered),
	)
}

func permissionModeLabel(mode permission.Mode) string {
	switch mode {
	case permission.ModeApprove:
		return "Approve for me"
	case permission.ModeFullAccess:
		return "Full Access"
	default:
		return "Ask for approval"
	}
}

func permissionSnapshot(runner Runner) permission.Snapshot {
	if runtime, ok := runner.(permissionRuntime); ok {
		return runtime.PermissionSnapshot()
	}
	return permission.NewState(permission.ModeAsk, false).Snapshot()
}

func setPermissionMode(runner Runner, mode permission.Mode, remembered bool) {
	if runtime, ok := runner.(permissionRuntime); ok {
		runtime.SetPermissionMode(mode, remembered)
	}
}

func activateFullAccess(runner Runner, remember bool) {
	if runtime, ok := runner.(permissionRuntime); ok {
		runtime.ActivateFullAccess(remember)
	}
}

func clearPermissionGrants(runner Runner) int {
	if runtime, ok := runner.(permissionRuntime); ok {
		return runtime.ClearPermissionGrants()
	}
	return 0
}

func (m Model) savePermissionSettings(next settings.PermissionSettings) error {
	if strings.TrimSpace(m.homeDir) == "" {
		return fmt.Errorf("home directory unavailable")
	}
	loaded, err := settings.Load(m.homeDir)
	if err != nil {
		return err
	}
	loaded.TUI.Permissions = next
	return settings.Save(m.homeDir, loaded)
}

func (m Model) saveEffortSetting(value string) error {
	if strings.TrimSpace(m.homeDir) == "" {
		return fmt.Errorf("home directory unavailable")
	}
	loaded, err := settings.Load(m.homeDir)
	if err != nil {
		return err
	}
	if err := settings.SetEffort(loaded, m.providerProtocol, value); err != nil {
		return err
	}
	return settings.Save(m.homeDir, loaded)
}

func commandArgsAfter(text string, command string, subcommand string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), command))
	return strings.TrimSpace(strings.TrimPrefix(rest, subcommand))
}

func (m Model) saveTUISettings() error {
	if strings.TrimSpace(m.homeDir) == "" {
		return fmt.Errorf("home directory unavailable")
	}
	loaded, err := settings.Load(m.homeDir)
	if err != nil {
		return err
	}
	loaded.TUI.Theme = m.themeName
	loaded.TUI.Statusline = append([]string(nil), m.statuslineItems...)
	loaded.TUI.Permissions = settings.PermissionSettings{
		Mode:                        string(m.permissionSnapshot.SelectedMode),
		FullAccessWarningRemembered: m.permissionSnapshot.FullAccessRemembered,
	}
	return settings.Save(m.homeDir, loaded)
}

func (m Model) openRewindSelector() (tea.Model, tea.Cmd) {
	if m.running {
		m.status = "Rewind unavailable while a run is active"
		return m, nil
	}
	records, err := m.runner.MessageHistory()
	if err != nil {
		m.appendEntry("error", "rewind", err.Error(), true)
		m.status = "Rewind unavailable"
		return m, nil
	}
	messages := checkpoint.SelectableMessages(records)
	if len(messages) == 0 {
		m.status = "No rewind targets"
		return m, nil
	}
	model := selector.New(messages, m.checkpointer)
	m.rewindSelector = &model
	m.status = "Rewind"
	return m, nil
}

// executePromptCommand kicks off file-based prompt command execution.
//
// The actual exec.Execute call — which may block for many seconds (shell
// embedding, before-hook) or many minutes (fork-mode sub-agent) — is
// dispatched into a tea.Cmd goroutine so it does not freeze the Bubble
// Tea event loop. The model is marked `running` immediately and the
// cancel function is wired up so Ctrl+C aborts the prepare stage as
// well as the eventual run.
func (m Model) executePromptCommand(cmd *slash.Command, args string, displayPrompt string) (tea.Model, tea.Cmd) {
	exec := m.slashExecutor
	if exec == nil {
		exec = slash.NewExecutor()
	}
	m.scrollOffset = 0
	m.running = true
	m.runStartedAt = m.nowTime()
	m.spinnerFrame = 0
	m.status = "Preparing skill " + cmd.Name
	runCtx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel
	sessionID := m.runner.SessionID()
	return m, executePromptCommandCmd(runCtx, exec, cmd, args, sessionID, displayPrompt, m.collaborationMode)
}

// promptCommandReadyMsg is emitted by the executor goroutine once
// exec.Execute has produced an ExecutionResult (or an error). The
// Update loop dispatches this to handlePromptCommandReady which decides
// whether to start an inline run, render a fork report, or surface an
// error.
type promptCommandReadyMsg struct {
	cmdName           string
	displayPrompt     string
	collaborationMode collaboration.Mode
	result            slash.ExecutionResult
	err               error
}

func executePromptCommandCmd(ctx context.Context, exec *slash.Executor, cmd *slash.Command, args, sessionID, displayPrompt string, mode collaboration.Mode) tea.Cmd {
	return func() tea.Msg {
		mode = collaboration.Normalize(mode)
		if mode == collaboration.ModeFormalPlan {
			switch {
			case strings.EqualFold(strings.TrimSpace(cmd.Frontmatter.Context), "fork"):
				return promptCommandReadyMsg{
					cmdName:           cmd.Name,
					displayPrompt:     displayPrompt,
					collaborationMode: mode,
					err:               fmt.Errorf("fork-mode prompt commands are unavailable in Formal Plan mode; use /plan off before running this command"),
				}
			case cmd.RunsShellAroundAgent():
				return promptCommandReadyMsg{
					cmdName:           cmd.Name,
					displayPrompt:     displayPrompt,
					collaborationMode: mode,
					err:               fmt.Errorf("prompt commands with embedded shell or shell hooks are unavailable in Formal Plan mode; use /plan off before running this command"),
				}
			}
		}
		res, err := exec.Execute(ctx, cmd, args, sessionID)
		return promptCommandReadyMsg{
			cmdName:           cmd.Name,
			displayPrompt:     displayPrompt,
			collaborationMode: mode,
			result:            res,
			err:               err,
		}
	}
}

func (m Model) handlePromptCommandReady(msg promptCommandReadyMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.running = false
		m.runStartedAt = time.Time{}
		m.cancelRun = nil
		m.appendEntry("error", "command failed", msg.err.Error(), true)
		m.status = "Command failed"
		return m, nil
	}
	if msg.result.Fork {
		// Fork-mode commands return a sub-agent report directly. Show
		// it as an assistant entry rather than starting a new turn so
		// the model is not asked to act on its own report. The exec
		// goroutine already ran before/after hooks synchronously, so
		// nothing more to drive.
		m.running = false
		m.runStartedAt = time.Time{}
		m.cancelRun = nil
		body := strings.TrimSpace(msg.result.Content)
		if body == "" {
			m.status = "Skill produced empty report"
			return m, nil
		}
		m.appendEntry("assistant", "skill", body, false)
		m.status = "Skill complete"
		return m, nil
	}
	if strings.TrimSpace(msg.result.Content) == "" {
		m.running = false
		m.runStartedAt = time.Time{}
		m.cancelRun = nil
		m.status = "Command produced empty output"
		return m, nil
	}
	// Inline mode: transition from prepare stage to actual run stage.
	// The prepare-stage runCtx is no longer relevant — derive a fresh
	// run context so Ctrl+C cancellation maps to the run, not to the
	// already-finished prepare phase.
	return m.runInlinePromptCommand(msg.result, msg.displayPrompt, msg.collaborationMode)
}

func (m Model) runInlinePromptCommand(result slash.ExecutionResult, displayPrompt string, mode collaboration.Mode) (tea.Model, tea.Cmd) {
	text := result.Content
	runCtx, cancel := context.WithCancel(m.ctx)
	m.cancelRun = cancel
	if strings.TrimSpace(displayPrompt) == "" {
		displayPrompt = text
	}
	m.appendEntry("user", "you", displayPrompt, false)
	runEffort := strings.TrimSpace(result.Effort)
	if runEffort != "" {
		normalized, err := effort.Validate(m.providerProtocol, runEffort)
		if err != nil {
			m.running = false
			m.runStartedAt = time.Time{}
			m.cancelRun = nil
			m.appendEntry("error", "command", err.Error(), true)
			m.status = "Invalid effort"
			return m, nil
		}
		runEffort = normalized
	}

	if len(result.AllowedTools) > 0 {
		rr, ok := m.runner.(restrictedRunner)
		if !ok {
			m.running = false
			m.runStartedAt = time.Time{}
			m.cancelRun = nil
			m.appendEntry("error", "command", "Runner does not support allowed-tools enforcement; aborting to avoid silent escape.", true)
			m.status = "Restricted run unsupported"
			return m, nil
		}
		m.status = "Running (restricted)"
		allowedCopy := append([]string(nil), result.AllowedTools...)
		if runEffort != "" {
			effortRunner, ok := rr.(restrictedEffortRunner)
			if !ok {
				m.running = false
				m.runStartedAt = time.Time{}
				m.cancelRun = nil
				m.appendEntry("error", "command", "Runner does not support effort override with allowed-tools; aborting to avoid silent escape.", true)
				m.status = "Effort run unsupported"
				return m, nil
			}
			return m, runRestrictedPromptCmdWithEffort(runCtx, effortRunner, text, displayPrompt, allowedCopy, runEffort, mode, result.AfterHook, m.events)
		}
		return m, runRestrictedPromptCmd(runCtx, rr, text, displayPrompt, allowedCopy, mode, result.AfterHook, m.events)
	}
	m.status = "Running"
	if runEffort != "" {
		effortRunner, ok := m.runner.(effortRunner)
		if !ok {
			m.running = false
			m.runStartedAt = time.Time{}
			m.cancelRun = nil
			m.appendEntry("error", "command", "Runner does not support effort override; aborting to avoid silently ignoring command effort.", true)
			m.status = "Effort run unsupported"
			return m, nil
		}
		return m, runPromptCmdWithEffortAndAfter(runCtx, effortRunner, text, displayPrompt, runEffort, mode, result.AfterHook, m.events)
	}
	return m, runPromptCmdWithAfter(runCtx, m.runner, text, displayPrompt, mode, result.AfterHook, m.events)
}

// restrictedRunner is the optional interface a Runner implements to
// support per-prompt tool restrictions. The production *AgentRunner
// satisfies it; test mocks may omit it and the TUI degrades gracefully
// to a hard error so allowed-tools is never silently ignored.
type restrictedRunner interface {
	RunRestrictedInCollaborationMode(ctx context.Context, prompt string, allowedTools []string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error)
}

type restrictedDisplayRunner interface {
	RunRestrictedWithDisplayInCollaborationMode(ctx context.Context, prompt string, displayPrompt string, allowedTools []string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error)
}

type effortRunner interface {
	RunWithDisplayAndEffortInCollaborationMode(ctx context.Context, prompt string, displayPrompt string, effort string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error)
}

type restrictedEffortRunner interface {
	RunRestrictedWithDisplayAndEffortInCollaborationMode(ctx context.Context, prompt string, displayPrompt string, allowedTools []string, effort string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error)
}

type collaborationDisplayRunner interface {
	RunWithDisplayInCollaborationMode(ctx context.Context, prompt string, displayPrompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error)
}

type projectInputHistoryRunner interface {
	ProjectInputHistory(limit int) ([]string, error)
}

func runRestrictedPromptCmd(ctx context.Context, runner restrictedRunner, prompt string, displayPrompt string, allowed []string, mode collaboration.Mode, afterHook func(context.Context), events chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		reporter := &channelReporter{events: events}
		var result *engine.RunResult
		var err error
		if displayRunner, ok := runner.(restrictedDisplayRunner); ok && strings.TrimSpace(displayPrompt) != "" {
			result, err = displayRunner.RunRestrictedWithDisplayInCollaborationMode(ctx, prompt, displayPrompt, allowed, mode, reporter)
		} else {
			result, err = runner.RunRestrictedInCollaborationMode(ctx, prompt, allowed, mode, reporter)
		}
		if afterHook != nil {
			afterHook(ctx)
		}
		return runFinishedMsg{result: result, err: err}
	}
}

func runRestrictedPromptCmdWithEffort(ctx context.Context, runner restrictedEffortRunner, prompt string, displayPrompt string, allowed []string, effort string, mode collaboration.Mode, afterHook func(context.Context), events chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		reporter := &channelReporter{events: events}
		result, err := runner.RunRestrictedWithDisplayAndEffortInCollaborationMode(ctx, prompt, displayPrompt, allowed, effort, mode, reporter)
		if afterHook != nil {
			afterHook(ctx)
		}
		return runFinishedMsg{result: result, err: err}
	}
}

func runPromptCmdWithAfter(ctx context.Context, runner Runner, prompt string, displayPrompt string, mode collaboration.Mode, afterHook func(context.Context), events chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		reporter := &channelReporter{events: events}
		var result *engine.RunResult
		var err error
		if displayRunner, ok := runner.(collaborationDisplayRunner); ok && strings.TrimSpace(displayPrompt) != "" {
			result, err = displayRunner.RunWithDisplayInCollaborationMode(ctx, prompt, displayPrompt, mode, reporter)
		} else {
			result, err = runner.RunInCollaborationMode(ctx, prompt, mode, reporter)
		}
		if afterHook != nil {
			afterHook(ctx)
		}
		return runFinishedMsg{result: result, err: err}
	}
}

func runPromptCmdWithEffortAndAfter(ctx context.Context, runner effortRunner, prompt string, displayPrompt string, effort string, mode collaboration.Mode, afterHook func(context.Context), events chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		reporter := &channelReporter{events: events}
		result, err := runner.RunWithDisplayAndEffortInCollaborationMode(ctx, prompt, displayPrompt, effort, mode, reporter)
		if afterHook != nil {
			afterHook(ctx)
		}
		return runFinishedMsg{result: result, err: err}
	}
}

func (m Model) handleSelectorResult(msg selector.ResultMsg) (tea.Model, tea.Cmd) {
	m.rewindSelector = nil
	if msg.Action == selector.ActionCancelled || msg.Action == selector.ActionNone {
		m.status = "Rewind cancelled"
		return m, nil
	}

	seq, err := strconv.ParseInt(msg.MessageID, 10, 64)
	if err != nil {
		m.appendEntry("error", "rewind", err.Error(), true)
		m.status = "Rewind failed"
		return m, nil
	}

	records, err := m.runner.MessageHistory()
	if err != nil {
		m.appendEntry("error", "rewind", err.Error(), true)
		m.status = "Rewind failed"
		return m, nil
	}
	content := messageContentBySeq(records, seq)

	if msg.Action == selector.ActionRestoreBoth || msg.Action == selector.ActionRestoreCode {
		if m.checkpointer != nil {
			files, err := m.checkpointer.Rewind(msg.MessageID)
			if err != nil {
				m.appendEntry("error", "rewind files", err.Error(), true)
				m.status = "Code restore failed"
				if msg.Action == selector.ActionRestoreCode {
					return m, nil
				}
			} else {
				m.appendCommandEntry("Rewind files", fmt.Sprintf("Restored %d file%s.", len(files), pluralS(len(files))))
			}
		}
	}

	if msg.Action == selector.ActionRestoreBoth || msg.Action == selector.ActionRestoreConversation {
		if err := m.restoreConversation(seq, content); err != nil {
			m.appendEntry("error", "rewind conversation", err.Error(), true)
			m.status = "Conversation restore failed"
			return m, nil
		}
		restored, err := m.runner.RestoreSessionStateBeforeMessage(seq)
		if err != nil {
			m.appendEntry("error", "rewind session state", err.Error(), true)
			m.status = "Session state restore failed"
			return m, nil
		}
		if restored {
			m.appendCommandEntry("Rewind session state", "Restored PLAN.md and TODO.md.")
		} else {
			m.appendCommandEntry("Rewind session state", "No PLAN.md/TODO.md snapshot found.")
		}
	}

	m.status = "Rewind complete"
	return m, nil
}

func (m *Model) restoreConversation(seq int64, content string) error {
	if err := m.runner.TruncateMessageHistory(seq); err != nil {
		return err
	}
	records, err := m.runner.MessageHistory()
	if err != nil {
		return err
	}
	m.entries = entriesFromMessageHistory(records)
	m.cachedLayout = nil
	if len(m.entries) == 0 {
		m.entries = []entry{sessionStartedEntry()}
	}
	m.inputHistory = inputHistoryFromMessageHistory(records)
	m.input = []rune(content)
	m.inputCursor = len(m.input)
	m.clearInputPastePreview()
	m.resetHistoryNavigation()
	m.resetCompletions()
	m.scrollOffset = 0
	return nil
}

func messageContentBySeq(records []session.MessageRecord, seq int64) string {
	for _, record := range records {
		if record.Seq == seq {
			return strings.TrimSpace(record.HumanContent())
		}
	}
	return ""
}

func (m *Model) tryAutoRestoreAfterCancel() {
	records, err := m.runner.MessageHistory()
	if err != nil {
		return
	}
	index := -1
	var target checkpoint.SelectableMessage
	for i := len(records) - 1; i >= 0; i-- {
		messages := checkpoint.SelectableMessages(records[i : i+1])
		if len(messages) == 0 {
			continue
		}
		index = i
		target = messages[0]
		break
	}
	if index < 0 || !checkpoint.MessagesAfterAreOnlySynthetic(records, index) {
		return
	}
	if err := m.restoreConversation(target.Seq, target.Content); err != nil {
		m.appendEntry("error", "auto restore", err.Error(), true)
		return
	}
	m.status = "Cancelled; restored input"
}

func isModelCommand(text string) bool {
	fields := strings.Fields(text)
	return len(fields) > 0 && strings.ToLower(fields[0]) == "/model"
}

func isQueuedModelCommand(text string) bool {
	fields := strings.Fields(text)
	return len(fields) > 1 && strings.ToLower(fields[0]) == "/model"
}

func (m Model) matchingSlashCommands() []slashCommand {
	text := string(m.input)
	if !strings.HasPrefix(text, "/") || strings.ContainsAny(text, " \t\n") {
		return nil
	}
	all := append([]slashCommand(nil), slashCommands...)
	all = append(all, m.fileBasedSlashCommands()...)

	if text == "/" {
		return uniqueSlashCommands(all)
	}
	query := strings.TrimPrefix(text, "/")
	var ranked []scoredSlash
	for i, command := range all {
		name := strings.TrimPrefix(command.Name, "/")
		s := slash.Score(query, name, command.Description, nil)
		if s > 0 {
			ranked = append(ranked, scoredSlash{command, s, i})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].order < ranked[j].order
	})
	matches := make([]slashCommand, len(ranked))
	for i, r := range ranked {
		matches[i] = r.cmd
	}
	return uniqueSlashCommands(matches)
}

type scoredSlash struct {
	cmd   slashCommand
	score int
	order int
}

func uniqueSlashCommands(in []slashCommand) []slashCommand {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := make([]slashCommand, 0, len(in))
	for _, c := range in {
		if seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		out = append(out, c)
	}
	return out
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
	lines = append(lines, "", "!<command>  run a local shell command without sending it to the model")
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

func (m Model) formatStatusOverview() string {
	provider, profile := m.providerStatusFields()
	groups := []statusGroup{
		{
			name: "Session",
			rows: []statusRow{
				{label: "ID", value: unavailableIfEmpty(m.runner.SessionID())},
				{label: "Dir", value: unavailableIfEmpty(m.runner.SessionDir())},
				{label: "Workdir", value: unavailableIfEmpty(m.runner.WorkDir())},
				{label: "Git", value: unavailableIfEmpty(m.gitBranch)},
			},
		},
		{
			name: "Model",
			rows: []statusRow{
				{label: "Provider", value: provider},
				{label: "Profile", value: profile},
				{label: "Model", value: unavailableIfEmpty(m.runner.Model())},
				{label: "Plan Mode", value: onOff(m.collaborationMode.PlanEnabled())},
			},
		},
		{
			name: "Runtime",
			rows: []statusRow{
				{label: "Run State", value: m.runStateLabel()},
				{label: "Queued Prompts", value: strconv.Itoa(len(m.queuedPrompts))},
				{label: "Context", value: normalizeContextUsage(m.contextUsage)},
				{label: "Permissions", value: permissionModeLabel(m.permissionSnapshot.EffectiveMode)},
				{label: "Session Approvals", value: strconv.Itoa(m.permissionSnapshot.SessionGrantCount)},
			},
		},
		{
			name: "UI",
			rows: []statusRow{
				{label: "Theme", value: unavailableIfEmpty(m.themeName)},
				{label: "Statusline", value: strings.Join(m.statuslineItems, ", ")},
				{label: "Sidebar", value: m.sidebarStatusLabel()},
			},
		},
		{
			name: "Capabilities",
			rows: []statusRow{
				{label: "Rewind", value: enabledDisabled(m.checkpointer != nil)},
				{label: "File Slash", value: enabledDisabled(m.slashRegistry != nil)},
				{label: "Ask User", value: enabledDisabled(m.asker != nil)},
			},
		},
	}
	return renderStatusGroups(groups)
}

type statusGroup struct {
	name string
	rows []statusRow
}

type statusRow struct {
	label string
	value string
}

func renderStatusGroups(groups []statusGroup) string {
	sections := make([]string, 0, len(groups))
	for _, group := range groups {
		labelWidth := 0
		for _, row := range group.rows {
			if len(row.label) > labelWidth {
				labelWidth = len(row.label)
			}
		}
		lines := []string{group.name}
		for _, row := range group.rows {
			lines = append(lines, fmt.Sprintf("  %-*s  %s", labelWidth, row.label, row.value))
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func (m Model) providerStatusFields() (string, string) {
	provider := strings.TrimSpace(m.providerProtocol)
	if provider == "" {
		provider = strings.TrimSpace(m.providerID)
	}
	if provider == "" {
		provider = "unavailable"
	}
	profile := strings.TrimSpace(m.providerProfileID)
	if profile == "" && (strings.TrimSpace(m.providerID) != "" || strings.TrimSpace(m.providerProtocol) != "") {
		profile = "inline"
	}
	if profile == "" {
		profile = "unavailable"
	}
	return provider, profile
}

func (m Model) runStateLabel() string {
	if m.running {
		if strings.TrimSpace(m.status) != "" {
			return strings.TrimSpace(m.status)
		}
		return "running"
	}
	return "idle"
}

func (m Model) sidebarStatusLabel() string {
	if m.sidebarFocused {
		return "focused"
	}
	if m.sidebarVisible {
		return "shown"
	}
	return "hidden"
}

func unavailableIfEmpty(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unavailable"
	}
	return value
}

func enabledDisabled(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func onOff(enabled bool) string {
	if enabled {
		return "on"
	}
	return "off"
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
	inputHistory := projectInputHistoryOrFallback(runner, inputHistoryFromMessageHistory(records))
	if len(entries) == 0 {
		return []entry{sessionStartedEntry()}, inputHistory, "Ready"
	}
	return entries, inputHistory, "Resumed session: " + runner.SessionID()
}

func projectInputHistoryOrFallback(runner Runner, fallback []string) []string {
	phr, ok := runner.(projectInputHistoryRunner)
	if !ok {
		return fallback
	}
	history, err := phr.ProjectInputHistory(inputHistoryLimit)
	if err != nil {
		return fallback
	}
	return history
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
			content := record.HumanContent()
			if !isRenderableHistoryContent(content) {
				continue
			}
			entries = append(entries, entry{
				role:  "user",
				title: "you",
				body:  content,
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
		content := record.HumanContent()
		if msg.Role != schema.RoleUser || msg.ToolCallID != "" || !isRenderableHistoryContent(content) {
			continue
		}
		text := strings.TrimSpace(content)
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
	m.cachedLayout = nil
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
	return workingFrames[(m.spinnerFrame/4)%len(workingFrames)]
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

func (m Model) wheelScrollDelta() (Model, int) {
	now := m.now()
	const base = 3
	if m.wheelSpeed > 0 && now.Sub(m.lastWheelTime) < 100*time.Millisecond {
		m.wheelSpeed = min(m.wheelSpeed+1, 6)
	} else {
		m.wheelSpeed = 1
	}
	m.lastWheelTime = now
	return m, base * m.wheelSpeed
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

type pendingEscTimeoutMsg struct {
	id uint64
}

func pendingEscCmd(id uint64) tea.Cmd {
	return tea.Tick(pendingEscDelay, func(time.Time) tea.Msg {
		return pendingEscTimeoutMsg{id: id}
	})
}

type mouseTailTimeoutMsg struct {
	id uint64
}

func mouseTailCmd(id uint64) tea.Cmd {
	return tea.Tick(mouseTailDelay, func(time.Time) tea.Msg {
		return mouseTailTimeoutMsg{id: id}
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

type compactFinishedMsg struct {
	result *compaction.CompactResult
	err    error
}

type shellCommandFinishedMsg struct {
	command string
	result  tools.BashCommandResult
}

func runPromptCmd(ctx context.Context, runner Runner, prompt string, mode collaboration.Mode, events chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		reporter := &channelReporter{events: events}
		result, err := runner.RunInCollaborationMode(ctx, prompt, mode, reporter)
		return runFinishedMsg{result: result, err: err}
	}
}

func runShellCommandCmd(ctx context.Context, workDir string, command string) tea.Cmd {
	return func() tea.Msg {
		return shellCommandFinishedMsg{
			command: command,
			result:  tools.RunBashCommand(ctx, workDir, command, 0),
		}
	}
}

func formatShellCommandResult(result tools.BashCommandResult) string {
	output := truncateShellCommandOutput(result.Output)
	if result.Truncated && output != "" {
		output += shellCommandTruncationMarker()
	}
	if result.TimedOut {
		if output == "" {
			return "Command timed out after 30s."
		}
		return output + "\n\nCommand timed out after 30s."
	}
	if result.Err != nil {
		status := result.Err.Error()
		if result.ExitCode != 0 {
			status = fmt.Sprintf("exit status %d", result.ExitCode)
		}
		if output == "" {
			return status
		}
		return output + "\n\n" + status
	}
	if output == "" {
		return "Command completed with no output."
	}
	return output
}

func truncateShellCommandOutput(output string) string {
	if len(output) <= maxShellCommandOutputBytes {
		return output
	}
	cut := 0
	for i := range output {
		if i > maxShellCommandOutputBytes {
			break
		}
		cut = i
	}
	if cut == 0 {
		cut = maxShellCommandOutputBytes
	}
	return output[:cut] +
		shellCommandTruncationMarker()
}

func shellCommandTruncationMarker() string {
	return fmt.Sprintf("\n\n[output truncated to %d bytes]", maxShellCommandOutputBytes)
}

func newSessionCmd(ctx context.Context, runner Runner) tea.Cmd {
	return func() tea.Msg {
		sessionID, err := runner.NewSession(ctx)
		return newSessionFinishedMsg{sessionID: sessionID, err: err}
	}
}

func compactNowCmd(ctx context.Context, runner Runner, customInstructions string) tea.Cmd {
	return func() tea.Msg {
		result, err := runner.CompactNow(ctx, customInstructions)
		return compactFinishedMsg{result: result, err: err}
	}
}
