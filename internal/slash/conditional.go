package slash

import (
	"log"
	"path/filepath"
	"sync"

	"github.com/bmatcuk/doublestar/v4"
)

// ConditionalSkills holds dormant skills that are activated only when the
// engine touches a matching file path. Skills are keyed by name; the
// associated patterns are evaluated relative to the project root when
// CheckAndActivate is called.
type ConditionalSkills struct {
	mu        sync.Mutex
	skills    map[string]*conditionalEntry
	activated map[string]bool
}

type conditionalEntry struct {
	patterns []string
	command  *Command
}

// NewConditionalSkills returns an empty container ready for Add and
// CheckAndActivate calls.
func NewConditionalSkills() *ConditionalSkills {
	return &ConditionalSkills{
		skills:    make(map[string]*conditionalEntry),
		activated: make(map[string]bool),
	}
}

// Len returns the number of dormant (not-yet-activated) skills.
func (c *ConditionalSkills) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.skills)
}

// Add registers a skill keyed by name with the supplied path patterns.
// When an entry already exists under the same name, the higher
// CommandSource wins (project > user > builtin). Equal-precedence
// re-registration emits a warning and overwrites the previous entry —
// matching the behavior of Registry.registerLocked for non-conditional
// commands. The cmd value is returned by Take after activation.
func (c *ConditionalSkills) Add(name string, patterns []string, cmd *Command) {
	if cmd == nil || name == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.skills[name]; ok {
		switch {
		case existing.command.Source > cmd.Source:
			return
		case existing.command.Source == cmd.Source:
			log.Printf("[slash] conditional skill %q re-registered at same precedence — overwriting", name)
		default:
			log.Printf("[slash] conditional skill %q overridden by higher-precedence source (%v -> %v)", name, existing.command.Source, cmd.Source)
		}
	}
	c.skills[name] = &conditionalEntry{
		patterns: append([]string(nil), patterns...),
		command:  cmd,
	}
}

// CheckAndActivate evaluates filePath against every dormant skill's
// patterns. It returns the names of skills that match (and so should be
// activated). Once a skill has matched, subsequent calls do not return it
// again.
//
// Patterns are evaluated relative to projectRoot when possible; failing
// that, they are matched against the basename and absolute path as
// fallbacks for non-project-rooted inputs.
func (c *ConditionalSkills) CheckAndActivate(filePath, projectRoot string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.skills) == 0 || filePath == "" {
		return nil
	}

	candidates := candidatePaths(filePath, projectRoot)

	var activated []string
	for name, entry := range c.skills {
		if c.activated[name] {
			continue
		}
		if anyPatternMatches(entry.patterns, candidates) {
			c.activated[name] = true
			activated = append(activated, name)
		}
	}
	return activated
}

// Take returns and removes the entry for name. Returns nil if no skill is
// stored under that name.
func (c *ConditionalSkills) Take(name string) *Command {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.skills[name]
	if !ok {
		return nil
	}
	delete(c.skills, name)
	return entry.command
}

func candidatePaths(filePath, projectRoot string) []string {
	out := []string{filepath.ToSlash(filePath), filepath.ToSlash(filepath.Base(filePath))}
	if projectRoot != "" {
		if rel, err := filepath.Rel(projectRoot, filePath); err == nil && !startsWithParentPath(rel) {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	return out
}

func startsWithParentPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == ".." {
		return true
	}
	if len(rel) >= 3 && rel[:3] == "../" {
		return true
	}
	return false
}

func anyPatternMatches(patterns, candidates []string) bool {
	for _, pat := range patterns {
		normalizedPat := filepath.ToSlash(pat)
		for _, c := range candidates {
			ok, err := doublestar.Match(normalizedPat, c)
			if err == nil && ok {
				return true
			}
		}
	}
	return false
}
