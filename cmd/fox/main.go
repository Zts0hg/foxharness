// Package main is the entry point for the fox CLI agent.
//
// Usage:
//
//	fox
//	fox "start this task in the TUI"
//	fox exec "run this task once"
//	fox -p "run this task once"
//	echo "your task" | fox exec -
//
// Flags:
//
//	-workdir    Working directory (default: current directory)
//	-prompt     User task prompt
//	-p, -print  Print response and exit without TUI
//	-model      LLM model name (resolved from settings, FOX_MODEL env, or glm-4.5-air)
//	-provider   Provider protocol: openai or claude (default: "openai")
//	-thinking   Enable legacy per-turn Thinking mode
//	-plan       Enable Plan Mode (default: true)
//	-max-turns  Maximum number of agent turns (default: 20)
//	-session    Resume a specific session ID
//	-continue   Resume the latest CLI session
//	-new        Force creation of a new session (default behavior)
//	-tui        Start an interactive terminal UI
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/Zts0hg/foxharness/internal/app"
	"github.com/Zts0hg/foxharness/internal/settings"
)

type launchMode int

const (
	launchTUI launchMode = iota
	launchPrint
)

func main() {
	cfg, mode, err := parseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		if err == flag.ErrHelp {
			return
		}
		log.Fatal(err)
	}

	homeDir, _ := os.UserHomeDir()
	loaded, _ := settings.Load(homeDir)
	foxModelEnv := os.Getenv("FOX_MODEL")
	cfg.Model = settings.ResolveModel(cfg.Model, foxModelEnv, "glm-4.5-air", loaded)

	if mode == launchTUI {
		cfg.Prompt = strings.TrimSpace(cfg.Prompt)
		onSave := func(model string) error {
			current, _ := settings.Load(homeDir)
			current.Model = model
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

func parseArgs(args []string, output io.Writer) (app.CLIConfig, launchMode, error) {
	var cfg app.CLIConfig
	mode := launchTUI
	if len(args) > 0 && args[0] == "exec" {
		mode = launchPrint
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
	fs.StringVar(&cfg.Model, "model", "", "LLM model name (default: glm-4.5-air)")
	fs.StringVar(&cfg.Provider, "provider", "openai", "provider protocol: openai or claude")
	fs.BoolVar(&cfg.EnableThinking, "thinking", false, "enable legacy per-turn Thinking mode; disabled when Plan Mode succeeds")
	fs.BoolVar(&cfg.EnablePlanMode, "plan", true, "enable Plan Mode")
	fs.IntVar(&cfg.MaxTurns, "max-turns", 20, "maximum number of agent turns")
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
		fmt.Fprintln(output)
		fmt.Fprintln(output, "Options:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return cfg, mode, err
	}

	if printMode {
		mode = launchPrint
	}
	if cfg.Interactive && mode == launchPrint {
		return cfg, mode, fmt.Errorf("-tui/-interactive 不能和 exec 或 -p/-print 同时使用")
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
