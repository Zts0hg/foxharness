package configcmd

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/Zts0hg/foxharness/internal/llmconfig"
)

// Deps carries the dependencies injected into Run. Keeping them explicit lets the
// entry point stay unit-testable without a real terminal.
type Deps struct {
	HomeDir     string
	Env         llmconfig.EnvLookup
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	StdinFD     int // file descriptor used for no-echo secret reads
	Interactive bool
	NewProvider ProviderFactory
}

// Run is the entry point for the `fox config` subcommand. subArgs selects the
// action directly (add / list / default [id]); with no recognized action it opens
// an interactive menu.
func Run(ctx context.Context, deps Deps, subArgs []string) error {
	action := ""
	if len(subArgs) > 0 {
		action = subArgs[0]
	}

	// Only actions that drive the prompter (the add wizard, the menu, or choosing
	// a default) require an interactive terminal. Read-only actions such as
	// `list` and `default <id>` may run non-interactively.
	needsPrompt := action == "" || action == "add" || (action == "default" && len(subArgs) < 2)
	if needsPrompt && !deps.Interactive {
		fmt.Fprintln(stderrOrDefault(deps.Stderr), "fox config requires an interactive terminal; non-interactive configuration is not supported yet.")
		return fmt.Errorf("non-interactive terminal")
	}

	stdout := deps.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stdin := deps.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	env := deps.Env
	if env == nil {
		env = func(string) string { return "" }
	}

	wizard := &Wizard{
		HomeDir:     deps.HomeDir,
		Env:         env,
		Prompter:    newTerminalPrompter(stdin, stdout, deps.StdinFD),
		Out:         stdout,
		NewProvider: deps.NewProvider,
	}

	if action == "" {
		menu := []string{"Add a provider", "List providers", "Set default provider"}
		idx, err := wizard.Prompter.Choose("What do you want to do?", menu)
		if err != nil {
			return err
		}
		switch idx {
		case 0:
			action = "add"
		case 1:
			action = "list"
		case 2:
			action = "default"
		}
	}

	switch action {
	case "add":
		return wizard.AddProfile()
	case "list":
		return wizard.ListProfiles()
	case "default":
		id := ""
		if len(subArgs) > 1 {
			id = subArgs[1]
		}
		return wizard.SetDefault(id)
	default:
		return fmt.Errorf("unknown config action %q (expected add, list, or default)", action)
	}
}

func stderrOrDefault(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
