package automemory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		if entry.IsDir() || entry.Name() == indexFileName || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		mem, err := ParseMemory(data)
		if err != nil || mem.Validate() != nil {
			continue
		}
		memories = append(memories, mem)
	}
	sort.Slice(memories, func(i, j int) bool {
		return memories[i].Name < memories[j].Name
	})
	return memories, nil
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
