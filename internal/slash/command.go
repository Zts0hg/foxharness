package slash

// CommandType identifies whether a command is a hardcoded built-in handler
// or a file-based prompt command.
type CommandType int

const (
	// CommandBuiltin is a handler-driven command registered in Go code.
	// Builtins typically own TUI side effects (open selector, switch model)
	// and are not invocable by the model.
	CommandBuiltin CommandType = iota

	// CommandPrompt is a file-based command whose body becomes the user
	// prompt after argument substitution and the rest of the execution
	// pipeline have processed it.
	CommandPrompt
)

// CommandSource identifies where a command was loaded from. Lower values
// represent lower precedence: a project-level command overrides a user-level
// command with the same name, which in turn overrides a built-in.
type CommandSource int

const (
	// SourceBuiltin marks commands compiled into the binary.
	SourceBuiltin CommandSource = iota

	// SourceUser marks commands loaded from ~/.foxharness/.
	SourceUser

	// SourceProject marks commands loaded from the project's .foxharness/.
	SourceProject
)

// Frontmatter holds parsed YAML frontmatter from a .md command file.
//
// All fields are optional; the zero value represents a file with no
// frontmatter block. Defaults are applied by the discovery layer rather
// than by parsing — for example, an empty Description is later replaced by
// the first non-blank line of the content body.
type Frontmatter struct {
	Description            string            `yaml:"description"`
	Arguments              string            `yaml:"arguments"`
	ArgumentHint           string            `yaml:"argument-hint"`
	AllowedTools           []string          `yaml:"allowed-tools"`
	Model                  string            `yaml:"model"`
	Effort                 string            `yaml:"effort"`
	UserInvocable          bool              `yaml:"user-invocable"`
	DisableModelInvocation bool              `yaml:"disable-model-invocation"`
	WhenToUse              string            `yaml:"when_to_use"`
	Context                string            `yaml:"context"`
	Agent                  string            `yaml:"agent"`
	Paths                  []string          `yaml:"paths"`
	Aliases                []string          `yaml:"aliases"`
	Hooks                  *FrontmatterHooks `yaml:"hooks"`
	Version                string            `yaml:"version"`

	// userInvocableExplicit records whether UserInvocable was present in
	// the YAML source, so the loader can distinguish "unset" (default true)
	// from "explicitly false". Not parsed from YAML; populated by the
	// frontmatter loader.
	userInvocableExplicit bool
}

// FrontmatterHooks contains optional before/after shell hooks defined in
// a command's frontmatter.
type FrontmatterHooks struct {
	Before string `yaml:"before"`
	After  string `yaml:"after"`
}

// HandlerFunc is the in-process handler signature for built-in commands.
// The concrete return type (tea.Model, tea.Cmd) is kept opaque here so the
// slash package stays free of any TUI dependency. The TUI registers built-in
// handlers as Handler closures with the right types and casts on dispatch.
type HandlerFunc func(args []string) (any, any)

// Command is the unified representation of a slash command in the registry,
// regardless of whether it was registered as a built-in or loaded from disk.
//
// Built-in commands populate Handler; prompt commands populate Content,
// FilePath, SkillDir, and Frontmatter. The Type field selects which set of
// fields is authoritative.
type Command struct {
	Type        CommandType
	Name        string
	Description string
	Aliases     []string
	Source      CommandSource
	Hidden      bool

	Frontmatter Frontmatter
	Content     string
	FilePath    string
	SkillDir    string

	Handler HandlerFunc
}

// IsUserInvocable reports whether the command should be offered to the user
// via the TUI autocomplete and dispatch path. Hidden commands and commands
// whose frontmatter explicitly set user-invocable: false are excluded.
func (c Command) IsUserInvocable() bool {
	if c.Hidden {
		return false
	}
	return c.Frontmatter.UserInvocable
}

// IsModelInvocable reports whether the LLM agent may call this command
// through the `skill` tool. Built-in commands are never model-invocable.
// Prompt commands are invocable unless the frontmatter sets
// disable-model-invocation: true.
func (c Command) IsModelInvocable() bool {
	if c.Type != CommandPrompt {
		return false
	}
	return !c.Frontmatter.DisableModelInvocation
}

// MatchesAlias returns true if the supplied name exactly matches one of the
// command's declared aliases. Matching is case-sensitive.
func (c Command) MatchesAlias(name string) bool {
	for _, alias := range c.Frontmatter.Aliases {
		if alias == name {
			return true
		}
	}
	for _, alias := range c.Aliases {
		if alias == name {
			return true
		}
	}
	return false
}
