package slash

import (
	"log"
	"os"
	"sort"
	"sync"
)

const (
	cacheKeyAll            = "all"
	cacheKeyUserInvocable  = "user_invocable"
	cacheKeyModelInvocable = "model_invocable"
)

// Registry is the unified store of slash commands. It merges in-process
// built-ins with .md files discovered on disk under .foxharness/ and
// Claude-compatible .claude/ directories, enforcing precedence
// (project > user > builtin) and providing cached views for the hot
// autocomplete path.
//
// Registry is safe for concurrent reads but mutation should be serialized;
// in practice the TUI mutates the registry only at startup, during /refresh,
// and when a conditional skill is activated, so external locking is
// unnecessary.
type Registry struct {
	mu               sync.RWMutex
	workDir          string
	userHome         string
	commands         map[string]*Command
	conditional      *ConditionalSkills
	cache            *Cache
	builtinOnRefresh []*Command
	onActivate       func(*Command)
	skipDiscovery    bool
}

// NewRegistry creates an empty Registry bound to workDir. The user home for
// discovery defaults to the OS home directory; tests can override it with
// WithUserHome.
func NewRegistry(workDir string) *Registry {
	home, _ := os.UserHomeDir()
	return &Registry{
		workDir:     workDir,
		userHome:    home,
		commands:    make(map[string]*Command),
		conditional: NewConditionalSkills(),
		cache:       NewCache(),
	}
}

// WithUserHome overrides the user-home directory used by Load and Refresh.
// Primarily for tests; production callers should rely on the default.
func (r *Registry) WithUserHome(home string) *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.userHome = home
	return r
}

// WithoutDiscovery disables filesystem discovery during Load and Refresh.
// Useful for tests that only want to exercise the built-in registration
// path without producing temp directories.
func (r *Registry) WithoutDiscovery() *Registry {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skipDiscovery = true
	return r
}

// OnActivate registers a callback invoked when a conditional skill is moved
// from dormant to active. The TUI uses this to refresh autocomplete state
// and the engine to re-render the system prompt's skill list.
func (r *Registry) OnActivate(fn func(*Command)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onActivate = fn
}

// WorkDir returns the workspace directory the registry was constructed with.
func (r *Registry) WorkDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.workDir
}

// Register adds a command to the registry. Lower-precedence registrations
// of an existing name are ignored with a debug log; equal-or-higher
// precedence registrations overwrite (with a warning).
func (r *Registry) Register(cmd *Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerLocked(cmd)
	r.cache.Invalidate()
}

func (r *Registry) registerLocked(cmd *Command) {
	if cmd == nil || cmd.Name == "" {
		return
	}
	if len(cmd.Frontmatter.Paths) > 0 && cmd.Type == CommandPrompt {
		r.conditional.Add(cmd.Name, cmd.Frontmatter.Paths, cmd)
		return
	}
	r.activateLocked(cmd)
}

// activateLocked places cmd into the active command set if it is at least
// as high-precedence as any existing entry under the same name. It is
// shared by registerLocked and the conditional-activation path so both
// honor the same precedence rules (project > user > builtin) — without
// activateLocked, conditional activation would silently overwrite a
// higher-precedence active command.
//
// Returns true when cmd ended up in the active set.
func (r *Registry) activateLocked(cmd *Command) bool {
	existing, ok := r.commands[cmd.Name]
	if ok {
		if existing.Source > cmd.Source {
			log.Printf("[slash] command %q suppressed by higher-precedence active command (%v vs incoming %v)", cmd.Name, existing.Source, cmd.Source)
			return false
		}
		if existing.Source == cmd.Source {
			log.Printf("[slash] command %q re-registered at same precedence — overwriting", cmd.Name)
		} else {
			log.Printf("[slash] command %q overridden by higher-precedence source (%v -> %v)", cmd.Name, existing.Source, cmd.Source)
		}
	}
	r.commands[cmd.Name] = cmd
	return true
}

// Lookup finds a command by exact name or alias.
func (r *Registry) Lookup(name string) (*Command, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if cmd, ok := r.commands[name]; ok {
		return cmd, true
	}
	var best *Command
	for _, cmd := range r.commands {
		if !cmd.MatchesAlias(name) {
			continue
		}
		if best == nil || cmd.Source > best.Source || (cmd.Source == best.Source && cmd.Name < best.Name) {
			best = cmd
		}
	}
	return best, best != nil
}

// All returns every registered command, sorted by name. The returned slice
// is cached; callers must treat it as read-only.
func (r *Registry) All() []*Command {
	if cached, ok := r.cache.Get(cacheKeyAll); ok {
		return cached
	}
	r.mu.RLock()
	cmds := make([]*Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	r.mu.RUnlock()
	sort.SliceStable(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })
	r.cache.Set(cacheKeyAll, cmds)
	return cmds
}

// UserInvocable returns the subset of commands the user may invoke via the
// TUI autocomplete and dispatch.
func (r *Registry) UserInvocable() []*Command {
	if cached, ok := r.cache.Get(cacheKeyUserInvocable); ok {
		return cached
	}
	all := r.All()
	out := make([]*Command, 0, len(all))
	for _, cmd := range all {
		if cmd.IsUserInvocable() {
			out = append(out, cmd)
		}
	}
	r.cache.Set(cacheKeyUserInvocable, out)
	return out
}

// ModelInvocable returns the subset of commands the LLM may invoke through
// the `skill` tool.
func (r *Registry) ModelInvocable() []*Command {
	if cached, ok := r.cache.Get(cacheKeyModelInvocable); ok {
		return cached
	}
	all := r.All()
	out := make([]*Command, 0, len(all))
	for _, cmd := range all {
		if cmd.IsModelInvocable() {
			out = append(out, cmd)
		}
	}
	r.cache.Set(cacheKeyModelInvocable, out)
	return out
}

// Load performs initial discovery and registers every .md command found
// under the user-level and project-level .foxharness/ and .claude/
// directories. Built-in commands previously registered are preserved.
func (r *Registry) Load() error {
	r.mu.Lock()
	r.builtinOnRefresh = r.snapshotBuiltinLocked()
	skip := r.skipDiscovery
	workDir := r.workDir
	userHome := r.userHome
	r.mu.Unlock()

	if skip {
		return nil
	}
	userCmds, projectCmds, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cmd := range userCmds {
		r.registerLocked(cmd)
	}
	for _, cmd := range projectCmds {
		r.registerLocked(cmd)
	}
	r.cache.Invalidate()
	return nil
}

// Refresh re-discovers commands from disk, replacing all file-based entries
// while preserving the previously registered built-ins.
func (r *Registry) Refresh() error {
	r.mu.Lock()
	builtins := append([]*Command(nil), r.builtinOnRefresh...)
	for name, cmd := range r.commands {
		if cmd.Source == SourceBuiltin {
			builtins = appendUniqueBuiltin(builtins, cmd)
		}
		_ = name
	}
	r.commands = make(map[string]*Command)
	r.conditional = NewConditionalSkills()
	for _, b := range builtins {
		r.registerLocked(b)
	}
	skip := r.skipDiscovery
	workDir := r.workDir
	userHome := r.userHome
	r.mu.Unlock()

	if skip {
		r.cache.Invalidate()
		return nil
	}
	userCmds, projectCmds, err := DiscoverCommands(workDir, userHome)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, cmd := range userCmds {
		r.registerLocked(cmd)
	}
	for _, cmd := range projectCmds {
		r.registerLocked(cmd)
	}
	r.cache.Invalidate()
	return nil
}

func appendUniqueBuiltin(list []*Command, cmd *Command) []*Command {
	for _, c := range list {
		if c.Name == cmd.Name {
			return list
		}
	}
	return append(list, cmd)
}

func (r *Registry) snapshotBuiltinLocked() []*Command {
	out := make([]*Command, 0)
	for _, cmd := range r.commands {
		if cmd.Source == SourceBuiltin {
			out = append(out, cmd)
		}
	}
	return out
}

// CheckConditional invokes the conditional skill matcher with filePath and
// activates any newly-matched skills, returning their names.
func (r *Registry) CheckConditional(filePath string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	names := r.conditional.CheckAndActivate(filePath, r.workDir)
	if len(names) == 0 {
		return nil
	}
	activated := make([]string, 0, len(names))
	for _, name := range names {
		cmd := r.conditional.Take(name)
		if cmd == nil {
			continue
		}
		if !r.activateLocked(cmd) {
			continue
		}
		activated = append(activated, cmd.Name)
		if r.onActivate != nil {
			r.onActivate(cmd)
		}
	}
	r.cache.Invalidate()
	return activated
}

// HasConditional reports whether the registry holds any dormant
// conditional skills not yet activated.
func (r *Registry) HasConditional() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conditional.Len() > 0
}
