// Package slash provides the file-based slash command system for foxharness.
//
// Commands and skills are discovered from .foxharness/commands/,
// .foxharness/skills/, and Claude Code-compatible .claude/commands/ and
// .claude/skills/ directories at both user-level and project-level (search
// from cwd up to the git root). Each .md file becomes a command whose body is
// sent as a user prompt, with optional YAML frontmatter controlling metadata,
// argument substitution, allowed tools, model overrides, conditional
// activation, and fork-mode execution.
//
// Key Components:
//   - Command, CommandType, CommandSource, Frontmatter: core data types
//   - Registry: unified registry merging built-in and file-based commands
//     with precedence rules (project > user > builtin)
//   - DiscoverCommands: file discovery and loading from disk
//   - ParseFrontmatter: YAML frontmatter parsing with graceful degradation
//   - Executor: orchestrates the execution pipeline (arguments → shell →
//     variables → hooks → dispatch)
//   - ParseArguments / SubstituteArguments: shell-style parsing and
//     placeholder substitution
//   - FilteredRegistry: tools.Registry wrapper enforcing allowed-tools
//   - FuzzyScore / FilterCommands: weighted autocomplete scoring
//   - ConditionalSkills: paths-glob driven on-demand skill activation
//
// The package depends only on the standard library, gopkg.in/yaml.v3,
// github.com/bmatcuk/doublestar/v4, and internal/tools (for the Registry
// interface used by FilteredRegistry). It MUST NOT import internal/tui,
// internal/engine, or internal/subagent — fork-mode execution is delegated
// through the ForkRunner interface that is satisfied by an external adapter.
package slash
