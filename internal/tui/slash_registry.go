// Package tui — slash_registry.go bridges the file-based slash command
// system (internal/slash) into the existing TUI dispatch and autocomplete.
//
// The TUI keeps a hand-curated set of 10 built-in commands in the global
// `slashCommands` slice; this file adds a layer on top so that prompt-style
// commands discovered from .foxharness/ or Claude-compatible .claude/
// directories appear in the same autocomplete menu and dispatch through the
// same entry point. Built-in behavior is unchanged when no registry is
// attached.
package tui

import (
	"context"
	"strings"

	"github.com/Zts0hg/foxharness/internal/slash"
)

// WithRegistry attaches a slash command registry and executor to the model.
// Tests that do not need file-based commands may construct the model
// without ever calling this; the existing built-in switch dispatch keeps
// working in that mode.
func (m Model) WithRegistry(registry *slash.Registry, executor *slash.Executor) Model {
	m.slashRegistry = registry
	m.slashExecutor = executor
	return m
}

// SlashRegistry exposes the attached registry for callers (e.g. /refresh
// command or runtime introspection). May return nil.
func (m Model) SlashRegistry() *slash.Registry {
	return m.slashRegistry
}

// fileBasedSlashCommands returns the registry's user-invocable prompt
// commands as slashCommand entries suitable for the existing autocomplete
// renderer. Built-in commands tracked by the registry are dropped because
// they are already represented in the global slashCommands slice.
func (m Model) fileBasedSlashCommands() []slashCommand {
	if m.slashRegistry == nil {
		return nil
	}
	out := make([]slashCommand, 0)
	for _, cmd := range m.slashRegistry.UserInvocable() {
		if cmd.Type == slash.CommandBuiltin {
			continue
		}
		name := "/" + cmd.Name
		out = append(out, slashCommand{Name: name, Description: cmd.Description})
	}
	return out
}

// lookupPromptCommand returns the registry-resident prompt command matching
// text (which must start with "/" and may include arguments). The bool
// return is false when no registry is attached, when text is not a
// recognized prompt command, or when the matched command is a builtin
// (which the existing switch dispatch handles).
func (m Model) lookupPromptCommand(text string) (*slash.Command, string, bool) {
	if m.slashRegistry == nil {
		return nil, "", false
	}
	fields := strings.Fields(text)
	if len(fields) == 0 || !strings.HasPrefix(fields[0], "/") {
		return nil, "", false
	}
	name := strings.TrimPrefix(fields[0], "/")
	args := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))
	cmd, ok := m.slashRegistry.Lookup(name)
	if !ok {
		return nil, "", false
	}
	if cmd.Type != slash.CommandPrompt {
		return nil, "", false
	}
	return cmd, args, true
}

// runPromptCommand processes a prompt command through the executor and
// returns the full ExecutionResult. The caller decides how to handle
// per-turn restrictions (e.g. allowed-tools) and the fork flag.
func (m Model) runPromptCommand(cmd *slash.Command, args string) (slash.ExecutionResult, error) {
	exec := m.slashExecutor
	if exec == nil {
		exec = slash.NewExecutor()
	}
	return exec.Execute(context.Background(), cmd, args, m.runner.SessionID())
}
