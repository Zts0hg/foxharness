// Package main is the entry point for the fox CLI agent.
//
// Usage:
//
//	fox
//	fox "start this task in the TUI"
//	fox exec "run this task once"
//	fox -p "run this task once"
//	echo "your task" | fox exec -
//	fox autodev [backlog-path]
//	fox render -scene transcript -out transcript.html
//	fox render -list
//
// Flags:
//
//	-workdir    Working directory (default: current directory)
//	-prompt     User task prompt
//	-p, -print  Print response and exit without TUI
//	-model      LLM model id override
//	-llm-provider  LLM provider profile id
//	-protocol   LLM provider protocol: openai or claude
//	-thinking   Enable legacy per-turn Thinking mode
//	-max-turns  Maximum number of agent turns; 0 means unlimited (default: 0)
//	-session    Resume a specific session ID
//	-continue   Resume the latest CLI session
//	-new        Force creation of a new session (default behavior)
//	-tui        Start an interactive terminal UI
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/childruntime"
	"github.com/Zts0hg/foxharness/internal/cli"
	"github.com/Zts0hg/foxharness/internal/configcmd"
	"github.com/Zts0hg/foxharness/internal/effort"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/llmresolve"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/session"
	"github.com/Zts0hg/foxharness/internal/settings"
	"github.com/Zts0hg/foxharness/internal/subagent"
	"github.com/Zts0hg/foxharness/internal/tui"

	"golang.org/x/term"
)

type launchMode int

const (
	launchTUI launchMode = iota
	launchPrint
	launchAutodev
)

func main() {
	args := os.Args[1:]
	if subArgs, ok := splitConfigArgs(args); ok {
		homeDir, _ := os.UserHomeDir()
		if err := runConfig(homeDir, subArgs); err != nil {
			exitWithError(err)
		}
		return
	}
	if subArgs, ok := splitRenderArgs(args); ok {
		if err := runRender(subArgs, os.Stdout); err != nil {
			exitWithError(err)
		}
		return
	}

	cfg, mode, err := parseArgs(args, os.Stderr)
	if err != nil {
		if err == flag.ErrHelp {
			return
		}
		log.Fatal(err)
	}

	homeDir, _ := os.UserHomeDir()
	resolvedLLM, err := resolveLLMConfig(homeDir, cfg.LLM, os.Getenv)
	if err != nil {
		if reportResolveError(err, os.Stderr) {
			os.Exit(1)
		}
		exitWithError(err)
	}
	cfg.ResolvedLLM = resolvedLLM
	cfg.Model = resolvedLLM.Model
	cfg.LLM.Model = resolvedLLM.Model
	applyPersistedEffort(homeDir, &cfg, resolvedLLM)
	cfg.NewChildRunner = newChildRunner
	if err := validateEffortConfig(&cfg, resolvedLLM); err != nil {
		exitWithError(err)
	}

	if mode == launchAutodev {
		// The positional argument, when present, is the backlog path.
		cfg.Prompt = strings.TrimSpace(cfg.Prompt)
		reporter := autodev.NewTerminalReporter(os.Stdout)
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		err := runAutodevWithSignals(context.Background(), signals, func(ctx context.Context) error {
			return runAutodev(ctx, cfg, reporter)
		})
		signal.Stop(signals)
		if err != nil {
			os.Exit(reportAutodevResult(err, os.Stderr))
		}
		return
	}

	if mode == launchTUI {
		cfg.Prompt = strings.TrimSpace(cfg.Prompt)
		onSave := func(model string) error {
			if resolvedLLM.SettingsProviderID == "" {
				return nil
			}
			current, _ := settings.Load(homeDir)
			if err := settings.SetProviderModel(current, resolvedLLM.SettingsProviderID, model); err != nil {
				log.Printf("[Settings] failed to update provider model: %v", err)
				return err
			}
			if err := settings.Save(homeDir, current); err != nil {
				log.Printf("[Settings] failed to save model: %v", err)
				return err
			}
			return nil
		}
		autodevLauncher := func(ctx context.Context, backlogPath string, reporter autodev.Reporter) error {
			return runAutodev(ctx, autodevConfigForTUILaunch(cfg, backlogPath), reporter)
		}
		if err := tui.Run(context.Background(), tui.Config{
			Model: cfg.Model, InitialPrompt: cfg.Prompt, HomeDir: homeDir,
			EffortOverride: cfg.EffortOverride, ProviderID: cfg.ResolvedLLM.ProviderID,
			ProviderProfileID: cfg.ResolvedLLM.SettingsProviderID,
			ProviderProtocol:  cfg.ResolvedLLM.Protocol,
			Autodev:           autodevLauncher,
			Initialize: func(ctx context.Context, interactions tui.Interactions) (tui.Startup, error) {
				return newTUIStartup(ctx, cfg, interactions, onSave)
			},
		}); err != nil {
			exitWithError(err)
		}
		return
	}

	prompt, err := readPrompt(cfg.Prompt)
	if err != nil {
		exitWithError(err)
	}

	cfg.Prompt = prompt
	if err := cli.Run(context.Background(), cli.Config{
		Prompt: cfg.Prompt,
		Initialize: func(ctx context.Context) (cli.Application, error) {
			return newCLIApplication(ctx, cfg)
		},
		Stdout: os.Stdout,
		Logger: log.Default(),
	}); err != nil {
		exitWithError(err)
	}
}

func newChildRunner(config app.ChildRunnerConfig) subagent.Runner {
	return childruntime.New(childruntime.Config{
		Provider: config.Provider, WorkDir: config.WorkDir,
		ParentProfile:    childruntime.ParentProfile(config.ParentProfile),
		ProviderProtocol: config.ProviderProtocol, Model: config.Model, Effort: config.Effort,
		Permission: config.Permission, ParentEvidence: config.ParentEvidence,
	})
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// splitConfigArgs reports whether args begins with the config subcommand and
// returns the remaining arguments as the config sub-arguments (add / list /
// default). The config subcommand is intercepted before flag parsing so its
// arguments are never treated as fox flags or prompt text.
func splitConfigArgs(args []string) ([]string, bool) {
	if len(args) > 0 && args[0] == "config" {
		return args[1:], true
	}
	return nil, false
}

// splitRenderArgs reports whether args begins with the render subcommand and
// returns the remaining arguments. The render subcommand renders a built-in TUI
// scene to a self-contained HTML snapshot for offline visual inspection; it is
// intercepted before flag parsing so its arguments are never treated as fox
// flags or prompt text.
func splitRenderArgs(args []string) ([]string, bool) {
	if len(args) > 0 && args[0] == "render" {
		return args[1:], true
	}
	return nil, false
}

// runRender parses the render subcommand flags and writes a self-contained HTML
// snapshot of the selected TUI scene. Opening the file in any browser reproduces
// the frame faithfully — including CJK, box-drawing, and symbol glyphs — because
// the document embeds the Sarasa Mono SC font and the theme palette. With -list
// it prints the available scene names and returns without rendering.
func runRender(subArgs []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("fox render", flag.ContinueOnError)
	fs.SetOutput(stdout)

	var (
		sceneName string
		sessionID string
		workDir   string
		outPath   string
		width     int
		height    int
		list      bool
	)
	fs.StringVar(&sceneName, "scene", "transcript", "built-in scene to render")
	fs.StringVar(&sessionID, "session", "", "render a real session instead of a scene: a session id, its directory, or a messages.jsonl path")
	fs.StringVar(&workDir, "workdir", ".", "working directory used to resolve a -session id")
	fs.StringVar(&workDir, "C", ".", "working directory used to resolve a -session id")
	fs.StringVar(&outPath, "out", "", "output HTML path (default: <scene|session>.html)")
	fs.IntVar(&width, "width", 120, "terminal width in cells")
	fs.IntVar(&height, "height", 34, "terminal height in cells")
	fs.BoolVar(&list, "list", false, "list available scenes and exit")
	fs.Usage = func() {
		fmt.Fprintln(stdout, "Usage:")
		fmt.Fprintln(stdout, "  fox render [options]              render a built-in TUI scene to a self-contained HTML file")
		fmt.Fprintln(stdout, "  fox render -session <id|path>    render a real session's transcript instead of a scene")
		fmt.Fprintln(stdout, "  fox render -list                 list available scenes")
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Open the HTML in any browser to view the faithful render (CJK and symbols included).")
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(subArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if list {
		for _, name := range tui.SceneNames() {
			fmt.Fprintln(stdout, name)
		}
		return nil
	}

	var (
		html        string
		err         error
		defaultBase = sceneName
	)
	if strings.TrimSpace(sessionID) != "" {
		records, loadErr := loadSessionRecords(sessionID, workDir)
		if loadErr != nil {
			return loadErr
		}
		html = tui.RenderSessionHTML(renderConversationRecords(records), width, height)
		defaultBase = "session"
	} else {
		html, err = tui.RenderSceneHTML(sceneName, width, height)
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(outPath) == "" {
		outPath = defaultBase + ".html"
	}
	if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s\n", outPath)
	return nil
}

func renderConversationRecords(records []session.MessageRecord) []app.ConversationRecord {
	result := make([]app.ConversationRecord, len(records))
	for index, record := range records {
		calls := make([]app.ConversationToolCall, len(record.Message.ToolCalls))
		for callIndex, call := range record.Message.ToolCalls {
			calls[callIndex] = app.ConversationToolCall{ID: call.ID, Name: call.Name, Arguments: string(call.Arguments)}
		}
		result[index] = app.ConversationRecord{
			Sequence: record.Seq, Time: record.Time, Role: string(record.Message.Role),
			Content: record.Message.Content, DisplayContent: record.DisplayContent,
			ToolCallID: record.Message.ToolCallID, ToolCalls: calls,
			IsMeta: record.IsMeta, IsCompactSummary: record.IsCompactSummary,
			IsVisibleInTranscriptOnly: record.IsVisibleInTranscriptOnly,
		}
	}
	return result
}

// loadSessionRecords loads a session's model-visible message records for
// replay rendering. The value may be a session directory, a `messages.jsonl`
// path (a session directory is inferred from it), or a session id resolved
// against workDir.
func loadSessionRecords(value string, workDir string) ([]session.MessageRecord, error) {
	if info, err := os.Stat(value); err == nil {
		dir := value
		if !info.IsDir() {
			dir = filepath.Dir(value)
		}
		sess := &session.StoredSession{RootDir: dir}
		if _, statErr := os.Stat(sess.MessagesPath()); statErr != nil {
			return nil, fmt.Errorf("render -session %q: no messages.jsonl found (looked at %s)", value, sess.MessagesPath())
		}
		return session.NewMessageLog(sess).LoadRecords()
	}

	sess, err := session.NewFileStore(workDir).Open(session.ID(value))
	if err != nil {
		return nil, fmt.Errorf("render -session %q: not a readable path and not a known session id under %q: %w", value, workDir, err)
	}
	return session.NewMessageLog(sess).LoadRecords()
}

// runConfig builds the wizard dependencies from the real environment and runs
// the `fox config` subcommand.
func runConfig(homeDir string, subArgs []string) error {
	fd := int(os.Stdin.Fd())
	deps := configcmd.Deps{
		HomeDir:     homeDir,
		Env:         os.Getenv,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		StdinFD:     fd,
		Interactive: term.IsTerminal(fd),
		NewProvider: provider.NewProbeProvider,
	}
	return configcmd.Run(context.Background(), deps, subArgs)
}

// reportResolveError prints the first-run onboarding guidance and reports true
// when err is the empty-configuration sentinel, signalling that the caller
// should exit. All other resolution errors are left for the caller to surface.
func reportResolveError(err error, w io.Writer) bool {
	if errors.Is(err, llmconfig.ErrNoProviderConfigured) {
		fmt.Fprintln(w, configcmd.OnboardingMessage())
		return true
	}
	return false
}

// exitCodeForError maps an autodev run outcome to the documented exit
// codes: 0 backlog drained, 2 precondition failure, 1 unexpected error.
func exitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var signalErr *autodevSignalError
	if errors.As(err, &signalErr) {
		switch signalErr.Signal {
		case os.Interrupt:
			return 130
		case syscall.SIGTERM:
			return 143
		}
	}
	var pre *autodev.PreconditionError
	if errors.As(err, &pre) {
		return 2
	}
	return 1
}

func reportAutodevResult(err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintln(stderr, err)
	}
	return exitCodeForError(err)
}

type autodevSignalError struct {
	Signal os.Signal
	Err    error
}

func (e *autodevSignalError) Error() string {
	return fmt.Sprintf("autodev interrupted by %s: %v", e.Signal, e.Err)
}

func (e *autodevSignalError) Unwrap() error { return e.Err }

func runAutodevWithSignals(parent context.Context, signals <-chan os.Signal, run func(context.Context) error) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	done := make(chan struct{})
	observed := make(chan os.Signal, 1)
	go func() {
		select {
		case sig := <-signals:
			observed <- sig
			cancel()
		case <-done:
		}
	}()
	err := run(ctx)
	close(done)
	select {
	case sig := <-observed:
		return &autodevSignalError{Signal: sig, Err: err}
	default:
		return err
	}
}

func parseArgs(args []string, output io.Writer) (app.CLIConfig, launchMode, error) {
	var cfg app.CLIConfig
	mode := launchTUI
	if len(args) > 0 && args[0] == "exec" {
		mode = launchPrint
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "autodev" {
		mode = launchAutodev
		args = args[1:]
	}

	fs := flag.NewFlagSet("fox", flag.ContinueOnError)
	if output == nil {
		output = io.Discard
	}
	fs.SetOutput(output)

	printMode := false
	fs.StringVar(&cfg.WorkDir, "workdir", ".", "working directory")
	fs.StringVar(&cfg.WorkDir, "C", ".", "working directory")
	fs.StringVar(&cfg.Prompt, "prompt", "", "user task prompt")
	fs.StringVar(&cfg.Model, "model", "", "LLM model id override")
	fs.StringVar(&cfg.LLM.ProviderID, "llm-provider", "", "LLM provider profile id")
	fs.StringVar(&cfg.LLM.Protocol, "protocol", "", "LLM provider protocol: openai or claude")
	fs.StringVar(&cfg.LLM.BaseURL, "base-url", "", "LLM API base URL")
	fs.StringVar(&cfg.LLM.Auth, "auth", "", "LLM auth mode: api-key or none")
	fs.StringVar(&cfg.LLM.APIKeyEnv, "api-key-env", "", "environment variable containing the LLM API key")
	fs.StringVar(&cfg.LLM.APIKey, "api-key", "", "LLM API key value; prefer -api-key-env for routine use")
	fs.StringVar(&cfg.EffortOverride, "effort", "", "reasoning effort override for the resolved provider protocol")
	fs.BoolVar(&cfg.EnableThinking, "thinking", false, "enable legacy per-turn Thinking mode")
	fs.IntVar(&cfg.MaxTurns, "max-turns", 0, "maximum number of agent turns; 0 means unlimited")
	fs.StringVar(&cfg.SessionID, "session", "", "resume a specific session ID")
	fs.StringVar(&cfg.SessionID, "r", "", "resume a specific session ID")
	fs.BoolVar(&cfg.ContinueSession, "continue", false, "resume the latest CLI session")
	fs.BoolVar(&cfg.ContinueSession, "c", false, "resume the latest CLI session")
	fs.BoolVar(&cfg.NewSession, "new", false, "force creation of a new session")
	fs.BoolVar(&cfg.Interactive, "tui", false, "start an interactive terminal UI (default)")
	fs.BoolVar(&cfg.Interactive, "interactive", false, "start an interactive terminal UI (default)")
	fs.BoolVar(&printMode, "p", false, "print response and exit without TUI")
	fs.BoolVar(&printMode, "print", false, "print response and exit without TUI")
	fs.Usage = func() {
		fmt.Fprintln(output, "Usage:")
		fmt.Fprintln(output, "  fox [options] [prompt]       start the interactive TUI")
		fmt.Fprintln(output, "  fox exec [options] [prompt]  run once and print the result")
		fmt.Fprintln(output, "  fox -p [options] [prompt]    run once and print the result")
		fmt.Fprintln(output, "  echo \"prompt\" | fox exec -  read the one-shot prompt from stdin")
		fmt.Fprintln(output, "  fox autodev [backlog-path]   drain the backlog autonomously (SDD pipeline per item)")
		fmt.Fprintln(output, "  fox render [options]         render a built-in TUI scene to a self-contained HTML snapshot")
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return cfg, mode, err
	}
	cfg.LLM.Model = cfg.Model

	if printMode {
		if mode == launchAutodev {
			return cfg, mode, fmt.Errorf("-p/-print 不能和 autodev 同时使用")
		}
		mode = launchPrint
	}
	if cfg.Interactive && mode != launchTUI {
		return cfg, mode, fmt.Errorf("-tui/-interactive 不能和 exec、-p/-print 或 autodev 同时使用")
	}

	positionalArgs := fs.Args()
	positionalPrompt := strings.TrimSpace(strings.Join(positionalArgs, " "))
	if strings.TrimSpace(cfg.Prompt) != "" && positionalPrompt != "" {
		return cfg, mode, fmt.Errorf("不能同时使用 -prompt 和位置参数 prompt")
	}
	if mode == launchAutodev && len(positionalArgs) > 1 {
		return cfg, mode, fmt.Errorf("autodev 最多接受一个 backlog-path 位置参数")
	}
	if strings.TrimSpace(cfg.Prompt) == "" {
		cfg.Prompt = positionalPrompt
	}
	return cfg, mode, nil
}

func resolveLLMConfig(homeDir string, cli llmconfig.CLIOverrides, lookup llmconfig.EnvLookup) (llmconfig.ResolvedConfig, error) {
	return llmresolve.FromUserSettings(homeDir, cli, lookup)
}

func validateEffortConfig(cfg *app.CLIConfig, resolved llmconfig.ResolvedConfig) error {
	if cfg == nil || strings.TrimSpace(cfg.EffortOverride) == "" {
		return nil
	}
	normalized, err := effort.Validate(resolved.Protocol, cfg.EffortOverride)
	if err != nil {
		return err
	}
	cfg.EffortOverride = normalized
	return nil
}

func applyPersistedEffort(homeDir string, cfg *app.CLIConfig, resolved llmconfig.ResolvedConfig) {
	if cfg == nil || strings.TrimSpace(cfg.EffortOverride) != "" {
		return
	}
	loaded, err := settings.Load(homeDir)
	if err != nil || loaded == nil {
		return
	}
	value := strings.TrimSpace(loaded.LLM.Effort[strings.ToLower(strings.TrimSpace(resolved.Protocol))])
	if value != "" {
		cfg.EffortOverride = value
	}
}

func readPrompt(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input != "" && input != "-" {
		return input, nil
	}

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}

	prompt := strings.TrimSpace(string(data))
	if prompt == "" {
		return "", fmt.Errorf("prompt 不能为空，请使用位置参数、-prompt 或通过 stdin 输入")
	}

	return prompt, nil
}
