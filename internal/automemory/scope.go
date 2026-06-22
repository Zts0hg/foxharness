package automemory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Zts0hg/foxharness/internal/session"
)

// Scope identifies one of the two storage tiers.
type Scope int

const (
	// ScopeUserGlobal is the cross-project tier under ~/.foxharness/memory/.
	ScopeUserGlobal Scope = iota
	// ScopeProject is the per-project tier under
	// ~/.foxharness/projects/{key}/memory/.
	ScopeProject
)

// indexFileName is the generated entry-point index written in each scope.
const indexFileName = "MEMORY.md"

// ScopeForType maps a memory type to its storage scope: user memories are
// cross-project (user-global); project, feedback, and reference memories are
// per-project (REQ-002).
func ScopeForType(t MemoryType) Scope {
	if t == TypeUser {
		return ScopeUserGlobal
	}
	return ScopeProject
}

// Dirs resolves the on-disk directories for both memory scopes given a home
// directory and the active project working directory. The project key reuses
// session.EncodeProjectPath so it stays identical to the session storage key
// (PLD-2).
type Dirs struct {
	homeDir string
	workDir string
}

// NewDirs constructs a Dirs rooted at homeDir for the project at workDir. The
// workDir is normalized to an absolute cleaned path so the derived project key
// matches session.Manager (which absolutizes before keying) regardless of
// whether the caller passed a relative or absolute path.
func NewDirs(homeDir, workDir string) Dirs {
	return Dirs{homeDir: homeDir, workDir: absWorkDir(workDir)}
}

// absWorkDir returns the absolute cleaned form of workDir, mirroring
// session.cleanAbsPath so the project key stays in sync with session storage.
func absWorkDir(workDir string) string {
	if workDir == "" {
		return ""
	}
	if abs, err := filepath.Abs(workDir); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(workDir)
}

// Dir returns the absolute directory for the given scope.
func (d Dirs) Dir(scope Scope) string {
	if scope == ScopeUserGlobal {
		return filepath.Join(d.homeDir, ".foxharness", "memory")
	}
	key := session.EncodeProjectPath(d.workDir)
	return filepath.Join(d.homeDir, ".foxharness", "projects", key, "memory")
}

// DirForType returns the absolute directory a memory of type t is stored in.
func (d Dirs) DirForType(t MemoryType) string {
	return d.Dir(ScopeForType(t))
}

// IndexPath returns the path to a scope's generated MEMORY.md index.
func (d Dirs) IndexPath(scope Scope) string {
	return filepath.Join(d.Dir(scope), indexFileName)
}

// EnsureDir creates the scope directory if it does not already exist. It is
// idempotent and safe to call repeatedly.
func (d Dirs) EnsureDir(scope Scope) error {
	dir := d.Dir(scope)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create memory directory %s: %w", dir, err)
	}
	return nil
}

// FilePath resolves the absolute path of the memory file named by a slug within
// the given scope. The slug is validated against path traversal: it must be a
// single, non-empty path element with no separators or "..". A trailing ".md"
// is accepted and not doubled.
func (d Dirs) FilePath(scope Scope, name string) (string, error) {
	safe, err := safeFileName(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(d.Dir(scope), safe), nil
}

// validMemoryName validates that a memory slug is a single, safe path element
// with no separators or ".." traversal. It is the single source of truth for
// name safety, shared by Save (filename derivation) and Validate (parsing), so a
// memory written directly via write_file and later loaded can never advertise an
// unsafe link in the index.
func validMemoryName(name string) error {
	trimmed := canonicalMemoryName(name)
	if trimmed == "" {
		return fmt.Errorf("memory name must not be empty")
	}
	if strings.ContainsAny(trimmed, `/\`) || strings.Contains(trimmed, "..") {
		return fmt.Errorf("memory name %q must not contain path separators or %q", name, "..")
	}
	if trimmed != filepath.Base(trimmed) {
		return fmt.Errorf("memory name %q must be a single path element", name)
	}
	return nil
}

// canonicalMemoryName returns the frontmatter/file slug form without a trailing
// markdown extension. A trailing ".md" is accepted for direct file writes, but
// the in-memory representation stays extensionless so index links are stable.
func canonicalMemoryName(name string) string {
	return strings.TrimSuffix(strings.TrimSpace(name), ".md")
}

// safeFileName validates a memory slug and returns the corresponding "<name>.md"
// filename, rejecting any value that could escape the memory directory.
func safeFileName(name string) (string, error) {
	trimmed := canonicalMemoryName(name)
	if err := validMemoryName(trimmed); err != nil {
		return "", err
	}
	return trimmed + ".md", nil
}
