/*
Package entryfixture provides deterministic immutable-fixture support for
cross-entry characterization tests.
*/
package entryfixture

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ManifestFilename     = "manifest.json"
	BaselineStatusFrozen = "frozen"
	BaselineStatusOpen   = "not_frozen"
)

/* Manifest describes one versioned characterization fixture set. */
type Manifest struct {
	Version        string    `json:"version"`
	BaselineStatus string    `json:"baseline_status"`
	BaselineCommit string    `json:"baseline_commit,omitempty"`
	Fixtures       []Fixture `json:"fixtures"`
}

/* Fixture identifies one immutable file and its baseline semantics. */
type Fixture struct {
	Path          string   `json:"path"`
	SHA256        string   `json:"sha256"`
	SourceCommit  string   `json:"source_commit"`
	Source        string   `json:"source"`
	Semantics     string   `json:"semantics"`
	Normalization []string `json:"normalization,omitempty"`
}

/* Load reads a fixture manifest from root. */
func Load(root string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(root, ManifestFilename))
	if err != nil {
		return nil, fmt.Errorf("read fixture manifest: %w", err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode fixture manifest: %w", err)
	}
	if err := manifest.validate(); err != nil {
		return nil, err
	}
	return &manifest, nil
}

/* Verify checks fixture metadata and file integrity. */
func (m *Manifest) Verify(root string) error {
	if err := m.validate(); err != nil {
		return err
	}
	listed := make(map[string]struct{}, len(m.Fixtures))
	for _, fixture := range m.Fixtures {
		listed[filepath.ToSlash(filepath.Clean(filepath.FromSlash(fixture.Path)))] = struct{}{}
		path, err := secureExistingFile(root, fixture.Path)
		if err != nil {
			return fmt.Errorf("fixture %q: %w", fixture.Path, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read fixture %q: %w", fixture.Path, err)
		}
		sum := sha256.Sum256(data)
		actual := hex.EncodeToString(sum[:])
		if actual != fixture.SHA256 {
			return fmt.Errorf("fixture %q sha256 mismatch: got %s want %s", fixture.Path, actual, fixture.SHA256)
		}
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk fixture authority: %w", walkErr)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve fixture authority entry: %w", err)
		}
		relative = filepath.ToSlash(relative)
		if relative == ManifestFilename {
			return nil
		}
		if _, ok := listed[relative]; !ok {
			return fmt.Errorf("unlisted fixture %q", relative)
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("fixture authority entry is not a regular file: %q", relative)
		}
		return nil
	})
}

/* CopyFixture copies one fixture file into a test-owned destination root. */
func CopyFixture(root, relativePath, destinationRoot string) (string, error) {
	source, err := secureExistingFile(root, relativePath)
	if err != nil {
		return "", err
	}
	rel, err := cleanRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		return "", fmt.Errorf("create fixture destination root: %w", err)
	}
	destination := filepath.Join(destinationRoot, rel)
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", fmt.Errorf("create fixture destination: %w", err)
	}
	in, err := os.Open(source)
	if err != nil {
		return "", fmt.Errorf("open fixture source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", fmt.Errorf("create fixture copy: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(destination)
		return "", fmt.Errorf("copy fixture: %w", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(destination)
		return "", fmt.Errorf("close fixture copy: %w", err)
	}
	return destination, nil
}

/* SequenceClock returns deterministic timestamps at a fixed interval. */
type SequenceClock struct {
	mu      sync.Mutex
	current time.Time
	step    time.Duration
}

/* NewSequenceClock creates a deterministic clock. */
func NewSequenceClock(start time.Time, step time.Duration) *SequenceClock {
	return &SequenceClock{current: start, step: step}
}

/* Now returns the next deterministic timestamp. */
func (c *SequenceClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := c.current
	c.current = c.current.Add(c.step)
	return result
}

/* IDSequence returns deterministic prefixed identifiers. */
type IDSequence struct {
	mu     sync.Mutex
	prefix string
	next   uint64
}

/* NewIDSequence creates an identifier sequence. */
func NewIDSequence(prefix string, first uint64) *IDSequence {
	return &IDSequence{prefix: prefix, next: first}
}

/* Next returns the next identifier. */
func (s *IDSequence) Next() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := fmt.Sprintf("%s-%04d", s.prefix, s.next)
	s.next++
	return result
}

func (m *Manifest) validate() error {
	if m.Version == "" {
		return fmt.Errorf("fixture manifest version is required")
	}
	if m.BaselineStatus != BaselineStatusOpen && m.BaselineStatus != BaselineStatusFrozen {
		return fmt.Errorf("unsupported baseline_status %q", m.BaselineStatus)
	}
	if m.BaselineStatus == BaselineStatusFrozen && m.BaselineCommit == "" {
		return fmt.Errorf("baseline_commit is required when baseline_status is frozen")
	}
	if m.BaselineStatus == BaselineStatusFrozen && len(m.Fixtures) == 0 {
		return fmt.Errorf("frozen fixture authority requires at least one fixture")
	}
	seen := make(map[string]struct{}, len(m.Fixtures))
	previousPath := ""
	for i, fixture := range m.Fixtures {
		clean, err := cleanRelativePath(fixture.Path)
		if err != nil {
			return fmt.Errorf("fixture %d path: %w", i, err)
		}
		key := filepath.ToSlash(clean)
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("duplicate fixture path %q", fixture.Path)
		}
		seen[key] = struct{}{}
		if previousPath != "" && key < previousPath {
			return fmt.Errorf("fixture paths must be sorted: %q appears after %q", key, previousPath)
		}
		previousPath = key
		if len(fixture.SHA256) != sha256.Size*2 {
			return fmt.Errorf("fixture %q sha256 must contain %d hexadecimal characters", fixture.Path, sha256.Size*2)
		}
		if _, err := hex.DecodeString(fixture.SHA256); err != nil {
			return fmt.Errorf("fixture %q sha256 is not hexadecimal: %w", fixture.Path, err)
		}
		if fixture.SourceCommit == "" || fixture.Source == "" || fixture.Semantics == "" {
			return fmt.Errorf("fixture %q requires source_commit, source, and semantics", fixture.Path)
		}
		if m.BaselineStatus == BaselineStatusFrozen && fixture.SourceCommit != m.BaselineCommit {
			return fmt.Errorf("fixture %q source_commit must equal baseline_commit", fixture.Path)
		}
	}
	return nil
}

func cleanRelativePath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) {
		return "", fmt.Errorf("fixture path must be a non-empty relative path")
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fixture path escapes its root: %q", path)
	}
	return clean, nil
}

func secureExistingFile(root, relativePath string) (string, error) {
	rel, err := cleanRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve fixture root: %w", err)
	}
	candidate := filepath.Join(resolvedRoot, rel)
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve fixture path: %w", err)
	}
	relToRoot, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return "", fmt.Errorf("compare fixture path: %w", err)
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fixture path resolves outside its root: %q", relativePath)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat fixture: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("fixture is not a regular file: %q", relativePath)
	}
	return resolved, nil
}
