package automemory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Store is the persistence layer for typed memory files across the two scopes.
// It validates on write, regenerates indexes from the files on disk, and is
// resilient to malformed files (it skips them rather than failing).
type Store struct {
	dirs Dirs
}

// NewStore constructs a Store rooted at homeDir for the project at workDir.
func NewStore(homeDir, workDir string) *Store {
	return &Store{dirs: NewDirs(homeDir, workDir)}
}

// Dirs exposes the resolved scope directories, used by callers that need to
// detect writes into the memory directories (e.g. the mutual-exclusion tracker).
func (s *Store) Dirs() Dirs {
	return s.dirs
}

// UserGlobalDir returns the absolute user-global memory directory.
func (s *Store) UserGlobalDir() string {
	return s.dirs.Dir(ScopeUserGlobal)
}

// ProjectDir returns the absolute project-scoped memory directory.
func (s *Store) ProjectDir() string {
	return s.dirs.Dir(ScopeProject)
}

// Save validates the memory and writes it to the directory selected by its type
// (REQ-002). The write is atomic: content is written to a temporary file in the
// destination directory and then renamed into place, so a crash can never leave a
// partially written memory file.
func (s *Store) Save(m Memory) error {
	m.Name = canonicalMemoryName(m.Name)
	if err := m.Validate(); err != nil {
		return err
	}
	scope := ScopeForType(m.Type)
	if err := s.dirs.EnsureDir(scope); err != nil {
		return err
	}
	path, err := s.dirs.FilePath(scope, m.Name)
	if err != nil {
		return err
	}
	data, err := m.Marshal()
	if err != nil {
		return err
	}
	return atomicWrite(path, data)
}

// Load returns every valid memory in the scope, sorted by name. Files with
// malformed frontmatter and the generated index file are skipped so a single bad
// file never breaks injection (orphan-safe).
func (s *Store) Load(scope Scope) ([]Memory, error) {
	dir := s.dirs.Dir(scope)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read memory directory %s: %w", dir, err)
	}

	var memories []Memory
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		mem, ok := s.loadableMemory(scope, filepath.Join(dir, entry.Name()))
		if !ok {
			continue
		}
		memories = append(memories, mem)
	}
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Name < memories[j].Name
	})
	return memories, nil
}

// loadableMemory parses and validates a single memory file for a scope, applying
// the same consistency checks Load uses (valid frontmatter, type matching the
// scope, filename matching the frontmatter name). It returns the memory and true
// when the file is a loadable memory for the scope, else zero/false.
func (s *Store) loadableMemory(scope Scope, absPath string) (Memory, bool) {
	if !directMemoryFileInDir(s.dirs.Dir(scope), absPath) {
		return Memory{}, false
	}
	entryName := filepath.Base(absPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return Memory{}, false
	}
	mem, err := ParseMemory(data)
	if err != nil || mem.Validate() != nil {
		return Memory{}, false
	}
	if ScopeForType(mem.Type) != scope {
		return Memory{}, false
	}
	wantName, err := safeFileName(mem.Name)
	if err != nil || wantName != entryName {
		return Memory{}, false
	}
	return mem, true
}

// IsLoadableMemoryAt reports whether absPath is a valid, loadable memory file in
// whichever memory directory contains it. It is the predicate the write tracker
// uses to decide whether a successful memory-directory write actually produced a
// memory, so a botched write (bad frontmatter, wrong type for the scope, the
// index file) does not set the mutual-exclusion flag and suppress extraction.
func (s *Store) IsLoadableMemoryAt(absPath string) bool {
	var scope Scope
	switch {
	case pathWithin(s.dirs.Dir(ScopeUserGlobal), absPath):
		scope = ScopeUserGlobal
	case pathWithin(s.dirs.Dir(ScopeProject), absPath):
		scope = ScopeProject
	default:
		return false
	}
	_, ok := s.loadableMemory(scope, absPath)
	return ok
}

// Remove deletes the named memory from the scope. Removing a memory that does not
// exist is a no-op so an explicit "forget" is idempotent.
func (s *Store) Remove(scope Scope, name string) error {
	path, err := s.dirs.FilePath(scope, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove memory %s: %w", name, err)
	}
	return nil
}

// atomicWrite writes data to path via a temporary file in the same directory
// followed by a rename, so readers never observe a partial file.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".memory-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp memory file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("failed to write temp memory file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to close temp memory file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("failed to commit memory file: %w", err)
	}
	return nil
}
