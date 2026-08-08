package tui

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Zts0hg/foxharness/internal/checkpoint"
	"github.com/Zts0hg/foxharness/internal/collaboration"
	"github.com/Zts0hg/foxharness/internal/compaction"
	"github.com/Zts0hg/foxharness/internal/effort"
	"github.com/Zts0hg/foxharness/internal/engine"
	"github.com/Zts0hg/foxharness/internal/permission"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/tools"
	"github.com/Zts0hg/foxharness/internal/tui/selector"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// fixedSnapshotTime is the injected clock value used when rendering snapshot
// scenes. Pinning the clock keeps time-derived output (timestamps, elapsed
// durations) stable across runs.
var fixedSnapshotTime = time.Date(2026, time.August, 7, 14, 17, 0, 0, time.UTC)

// fixedSnapshotSpinnerFrame is the animation phase used for snapshot scenes so
// the spinner glyph does not vary between runs.
const fixedSnapshotSpinnerFrame = 0

// SnapshotBackground is the window background fill used when exporting a scene
// to an image. It matches the Monokai Pro dark background so image exports have
// a fixed, terminal-independent appearance.
const SnapshotBackground = "#2d2a2e"

// snapshotForeground is the default text color applied to unstyled cells during
// image export, matching the Monokai Pro dark foreground.
const snapshotForeground = "#fcfcfa"

// monokaiDarkPalette maps the 16 ANSI color indices (0-15) to the Monokai Pro
// dark palette. The default TUI theme (codex) styles most elements with ANSI
// palette indices, which an image renderer would otherwise paint with its own
// palette. Baking these to fixed truecolor keeps exports deterministic and
// terminal-independent.
var monokaiDarkPalette = [16]string{
	"#2d2a2e", "#ff6188", "#a9dc76", "#ffd866", "#fc9867", "#ab9df2", "#78dce8", "#fcfcfa",
	"#727072", "#ff6188", "#a9dc76", "#ffd866", "#fc9867", "#ab9df2", "#78dce8", "#fcfcfa",
}

var sgrPattern = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// RecolorForImage rewrites the 16-color ANSI SGR codes in a rendered frame to
// fixed Monokai Pro truecolor and pins the default foreground. The result is a
// deterministic, terminal-independent frame suitable for image export; layout
// and content are untouched.
func RecolorForImage(frame string) string {
	recolored := sgrPattern.ReplaceAllStringFunc(frame, func(seq string) string {
		params := sgrPattern.FindStringSubmatch(seq)[1]
		return "\x1b[" + convertSGRColorParams(params) + "m"
	})
	defaultFG := truecolorFG(snapshotForeground)
	recolored = strings.ReplaceAll(recolored, "\x1b[0m", "\x1b[0m"+defaultFG)
	return defaultFG + recolored
}

// convertSGRColorParams rewrites the 16-color foreground/background parameters
// of one SGR sequence to truecolor using the Monokai Pro palette, preserving
// every non-color parameter.
func convertSGRColorParams(params string) string {
	if params == "" {
		return "0"
	}
	out := make([]string, 0, 8)
	for _, p := range strings.Split(params, ";") {
		n, err := strconv.Atoi(p)
		if err != nil {
			out = append(out, p)
			continue
		}
		switch {
		case n >= 30 && n <= 37:
			out = append(out, truecolorParams("38", monokaiDarkPalette[n-30]))
		case n >= 90 && n <= 97:
			out = append(out, truecolorParams("38", monokaiDarkPalette[n-90+8]))
		case n >= 40 && n <= 47:
			out = append(out, truecolorParams("48", monokaiDarkPalette[n-40]))
		case n >= 100 && n <= 107:
			out = append(out, truecolorParams("48", monokaiDarkPalette[n-100+8]))
		case n == 39:
			out = append(out, truecolorParams("38", snapshotForeground))
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, ";")
}

func truecolorParams(layer string, hex string) string {
	r, g, b := hexRGB(hex)
	return fmt.Sprintf("%s;2;%d;%d;%d", layer, r, g, b)
}

func truecolorFG(hex string) string {
	r, g, b := hexRGB(hex)
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

func hexRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	r, _ := strconv.ParseInt(hex[0:2], 16, 0)
	g, _ := strconv.ParseInt(hex[2:4], 16, 0)
	b, _ := strconv.ParseInt(hex[4:6], 16, 0)
	return int(r), int(g), int(b)
}

// scene pairs a stable name with a builder that returns a Model in a fixed
// state. Builders must not depend on wall-clock time or external I/O so that
// RenderSceneANSI stays deterministic.
type scene struct {
	name  string
	build func() Model
}

// snapshotScenes is the ordered catalog of renderable TUI states. Add a new
// entry here to make a state available to `fox render`.
var snapshotScenes = []scene{
	{name: "transcript", build: buildTranscriptScene},
	{name: "sidebar", build: buildSidebarScene},
	{name: "permission-form", build: buildPermissionFormScene},
	{name: "approval-form", build: buildApprovalFormScene},
	{name: "plan-form", build: buildPlanFormScene},
	{name: "ask-form", build: buildAskFormScene},
	{name: "effort-form", build: buildEffortFormScene},
	{name: "selector", build: buildSelectorScene},
}

// SceneNames returns the available snapshot scene names in catalog order.
func SceneNames() []string {
	names := make([]string, len(snapshotScenes))
	for i, sc := range snapshotScenes {
		names[i] = sc.name
	}
	return names
}

// RenderSceneANSI builds the named scene at the given terminal size and returns
// its View() output, including ANSI styling. The result is deterministic: the
// scene is rendered with a fixed clock and a fixed spinner frame. An unknown
// name returns an error listing the available scenes.
func RenderSceneANSI(name string, width, height int) (string, error) {
	var sc *scene
	for i := range snapshotScenes {
		if snapshotScenes[i].name == name {
			sc = &snapshotScenes[i]
			break
		}
	}
	if sc == nil {
		return "", fmt.Errorf("unknown scene %q; available: %s", name, strings.Join(SceneNames(), ", "))
	}
	return renderModelANSI(sc.build(), width, height), nil
}

// RenderSessionANSI renders a real session's model-visible message records — as
// the TUI transcript would show them — to a deterministic ANSI frame. Records
// are converted with the same history→entry mapping the live TUI uses.
func RenderSessionANSI(records []session.MessageRecord, width, height int) string {
	m := baseSnapshotModel()
	if entries := entriesFromMessageHistory(records); len(entries) > 0 {
		m.entries = entries
	}
	return renderModelANSI(m, width, height)
}

// renderModelANSI applies the deterministic snapshot knobs to a Model and
// returns its View(). The default lipgloss renderer downgrades to a colorless
// profile when stdout is not a terminal, stripping the theme's truecolor
// styling; force truecolor for the render, then restore the previous profile.
func renderModelANSI(m Model, width, height int) string {
	prevProfile := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prevProfile)

	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m = updated.(Model)
	m.now = func() time.Time { return fixedSnapshotTime }
	m.spinnerFrame = fixedSnapshotSpinnerFrame
	return m.View()
}

// baseSnapshotModel constructs a Model backed by the static snapshot runner.
func baseSnapshotModel() Model {
	return NewModel(context.Background(), snapshotRunner{}, Config{})
}

func buildTranscriptScene() Model {
	m := baseSnapshotModel()
	m.entries = []entry{
		{role: "user", title: "you", body: "fix the sidebar layout bug where long titles overflow", time: fixedSnapshotTime},
		{role: "assistant", title: "foxharness", body: "I'll inspect the sidebar renderer and reproduce the overflow first.", time: fixedSnapshotTime},
		{role: "tool", title: "call bash", body: formatToolInvocation("bash", `{"command":"rg -n overflow internal/tui/sidebar.go"}`), time: fixedSnapshotTime},
		{role: "tool", title: "result bash", body: "internal/tui/sidebar.go:112: // TODO: clamp overflow", time: fixedSnapshotTime},
		{role: "assistant", title: "foxharness", body: "Found it — the title is not truncated to the panel width. Patching now.", time: fixedSnapshotTime},
	}
	return m
}

func buildSidebarScene() Model {
	m := baseSnapshotModel()
	m.sidebarVisible = true
	m.sidebarDocuments = loadSidebarDocuments(
		m.runner.WorkDir(),
		m.runner.SessionDir(),
		m.runner.AutoMemoryIndex(),
	)
	return m
}

func buildPermissionFormScene() Model {
	m := baseSnapshotModel()
	m.permissionForm = newPermissionForm(permission.NewState(permission.ModeAsk, false).Snapshot())
	return m
}

func buildApprovalFormScene() Model {
	m := baseSnapshotModel()
	m.approvalForm = newApprovalForm(permissionRequest{
		approval: permission.ApprovalRequest{
			Request: permission.Request{
				ToolName:  "bash",
				Action:    "run a shell command",
				Arguments: `{"command":"rm -rf build/"}`,
				CWD:       "/repo",
				Risk:      permission.RiskHigh,
			},
		},
	})
	return m
}

func buildPlanFormScene() Model {
	m := baseSnapshotModel()
	m.planForm = newPlanReviewForm(planReviewRequest{
		planMarkdown: "# Refactor sidebar layout\n\n1. Clamp long titles to panel width\n2. Add an overflow ellipsis\n3. Cover with a golden snapshot scene",
	})
	return m
}

func buildAskFormScene() Model {
	m := baseSnapshotModel()
	m.askForm = newAskForm(askRequest{
		questions: []tools.Question{
			{
				Header: "Approach",
				Prompt: "Which rendering approach should we use for snapshots?",
				Options: []tools.Option{
					{Label: "freeze", Description: "Faithful terminal-screenshot PNG via charmbracelet/freeze"},
					{Label: "pure-go", Description: "Self-built ANSI-to-PNG renderer, deterministic but approximate"},
				},
			},
		},
	})
	return m
}

func buildEffortFormScene() Model {
	m := baseSnapshotModel()
	options, err := effort.OptionsForProtocol(effort.ProtocolClaude)
	if err != nil {
		options = []string{effort.Auto}
	}
	m.effortForm = newEffortForm(effort.ProtocolClaude, options, "medium")
	return m
}

func buildSelectorScene() Model {
	m := baseSnapshotModel()
	sel := selector.New([]checkpoint.SelectableMessage{
		{Seq: 1, Content: "add render subcommand skeleton", Timestamp: fixedSnapshotTime},
		{Seq: 2, Content: "seed transcript and sidebar scenes", Timestamp: fixedSnapshotTime},
	}, nil)
	m.rewindSelector = &sel
	return m
}

// snapshotRunner is a static Runner implementation used to build offline TUI
// scenes for rendering. It performs no real work and returns fixed values; only
// the methods exercised while constructing and viewing a Model need meaningful
// output.
type snapshotRunner struct{}

func (snapshotRunner) RunInCollaborationMode(ctx context.Context, prompt string, mode collaboration.Mode, reporter engine.Reporter) (*engine.RunResult, error) {
	return nil, nil
}

func (snapshotRunner) NewSession(ctx context.Context) (string, error) { return "snapshot", nil }

func (snapshotRunner) SessionID() string { return "snapshot" }

func (snapshotRunner) SessionDir() string { return "" }

func (snapshotRunner) WorkDir() string { return "." }

func (snapshotRunner) AutoMemoryIndex() string {
	return "- Prefer edit_file over write_file for existing code\n- Interact in Chinese; keep code and specs in English"
}

func (snapshotRunner) Model() string { return "glm-4.5-air" }

func (snapshotRunner) SetModel(model string) error { return nil }

func (snapshotRunner) ContextUsage() string { return "12%" }

func (snapshotRunner) MessageHistory() ([]session.MessageRecord, error) { return nil, nil }

func (snapshotRunner) TruncateMessageHistory(seq int64) error { return nil }

func (snapshotRunner) RestoreSessionStateBeforeMessage(seq int64) (bool, error) { return false, nil }

func (snapshotRunner) Checkpointer() checkpoint.Checkpointer { return nil }

func (snapshotRunner) CollaborationMode() collaboration.Mode { return collaboration.Normalize("") }

func (snapshotRunner) SetCollaborationMode(mode collaboration.Mode) {}

func (snapshotRunner) CompactNow(ctx context.Context, customInstructions string) (*compaction.CompactResult, error) {
	return nil, nil
}
