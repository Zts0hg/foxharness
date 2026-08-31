package hermetic

import (
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
)

/* FileSystem is an in-memory filesystem with deterministic operation failures. */
type FileSystem struct {
	mu       sync.Mutex
	files    map[string][]byte
	failures map[string]error
}

/* NewFileSystem returns an empty controlled filesystem. */
func NewFileSystem() *FileSystem {
	return &FileSystem{files: make(map[string][]byte), failures: make(map[string]error)}
}

/* Fail configures one operation to fail until cleared with a nil error. */
func (f *FileSystem) Fail(operation string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.failures, operation)
		return
	}
	f.failures[operation] = err
}

/* WriteFile stores an independent byte copy. */
func (f *FileSystem) WriteFile(name string, data []byte, _ fs.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failures["write"]; err != nil {
		return err
	}
	f.files[cleanPath(name)] = append([]byte(nil), data...)
	return nil
}

/* ReadFile returns an independent byte copy. */
func (f *FileSystem) ReadFile(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failures["read"]; err != nil {
		return nil, err
	}
	data, ok := f.files[cleanPath(name)]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

/* Rename atomically changes one controlled path. */
func (f *FileSystem) Rename(oldPath, newPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failures["rename"]; err != nil {
		return err
	}
	oldPath = cleanPath(oldPath)
	newPath = cleanPath(newPath)
	data, ok := f.files[oldPath]
	if !ok {
		return fs.ErrNotExist
	}
	f.files[newPath] = data
	delete(f.files, oldPath)
	return nil
}

/* Remove deletes one controlled path. */
func (f *FileSystem) Remove(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.failures["remove"]; err != nil {
		return err
	}
	name = cleanPath(name)
	if _, ok := f.files[name]; !ok {
		return fs.ErrNotExist
	}
	delete(f.files, name)
	return nil
}

func cleanPath(name string) string {
	clean := filepath.ToSlash(filepath.Clean(name))
	clean = strings.TrimPrefix(clean, "./")
	if clean == "." || clean == "" || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}
