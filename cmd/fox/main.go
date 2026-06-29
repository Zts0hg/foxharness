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
//	-plan       Enable Plan Mode (default: true)
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
	"strings"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/autodev"
	"github.com/Zts0hg/foxharness/internal/configcmd"
	"github.com/Zts0hg/foxharness/internal/llmconfig"
	"github.com/Zts0hg/foxharness/internal/llmresolve"
	"github.com/Zts0hg/foxharness/internal/provider"
	"github.com/Zts0hg/foxharness/internal/settings"

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

	if mode == launchAutodev {
		// The positional argument, when present, is the backlog path.
		cfg.Prompt = strings.TrimSpace(cfg.Prompt)
		reporter := autodev.NewTerminalReporter(os.Stdout)
		if err := app.RunAutodev(context.Background(), cfg, reporter); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(exitCodeForError(err))
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
		if err := app.RunTUI(context.Background(), cfg, onSave); err != nil {
			exitWithError(err)
		}
		return
	}

	prompt, err := readPrompt(cfg.Prompt)
	if err != nil {
		exitWithError(err)
	}

	cfg.Prompt = prompt
	if err := app.RunCLI(context.Background(), cfg); err != nil {
		exitWithError(err)
	}
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
		NewProvider: provider.NewProvider,
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
	var pre *autodev.PreconditionError
	if errors.As(err, &pre) {
		return 2
	}
	return 1
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
	fs.BoolVar(&cfg.EnableThinking, "thinking", false, "enable legacy per-turn Thinking mode; disabled when Plan Mode succeeds")
	fs.BoolVar(&cfg.EnablePlanMode, "plan", true, "enable Plan Mode")
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

	positionalPrompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if strings.TrimSpace(cfg.Prompt) != "" && positionalPrompt != "" {
		return cfg, mode, fmt.Errorf("不能同时使用 -prompt 和位置参数 prompt")
	}
	if strings.TrimSpace(cfg.Prompt) == "" {
		cfg.Prompt = positionalPrompt
	}
	return cfg, mode, nil
}

func resolveLLMConfig(homeDir string, cli llmconfig.CLIOverrides, lookup llmconfig.EnvLookup) (llmconfig.ResolvedConfig, error) {
	return llmresolve.FromUserSettings(homeDir, cli, lookup)
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
