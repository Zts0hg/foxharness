package autodev

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
)

var featureNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// featureWorkspace is the sole authority for CodexSpec artifact access.
// Both roots retain directory handles, while explicit Lstat checks reject
// symlinks and special files that os.Root containment alone permits.
type featureWorkspace struct {
	workRoot    *os.Root
	featureRoot *os.Root
	featureDir  string
}

var authoritativeArtifactNames = []string{
	"requirements.md",
	"spec.md",
	"review-spec.md",
	"plan.md",
	"review-plan.md",
	"tasks.md",
	"review-tasks.md",
}

func validateFeatureDir(featureDir string) error {
	if featureDir == "" {
		return fmt.Errorf("feature directory is empty")
	}
	if strings.TrimSpace(featureDir) != featureDir {
		return fmt.Errorf("feature directory %q is not normalized", featureDir)
	}
	if strings.Contains(featureDir, `\`) || path.IsAbs(featureDir) || path.Clean(featureDir) != featureDir {
		return fmt.Errorf("feature directory %q is not a normalized relative path", featureDir)
	}
	parts := strings.Split(featureDir, "/")
	if len(parts) != 3 || parts[0] != ".codexspec" || parts[1] != "specs" {
		return fmt.Errorf("feature directory %q must match .codexspec/specs/<feature-name>", featureDir)
	}
	if !featureNameRE.MatchString(parts[2]) || parts[2] == "." || parts[2] == ".." {
		return fmt.Errorf("feature directory %q has a malformed feature name", featureDir)
	}
	return nil
}

func openFeatureWorkspace(workDir, featureDir string, create bool) (*featureWorkspace, error) {
	if err := validateFeatureDir(featureDir); err != nil {
		return nil, err
	}
	workRoot, err := os.OpenRoot(workDir)
	if err != nil {
		return nil, fmt.Errorf("open worktree root: %w", err)
	}
	w := &featureWorkspace{workRoot: workRoot, featureDir: featureDir}
	for _, dir := range []string{".codexspec", ".codexspec/specs", featureDir} {
		if err := ensureRootedDirectory(workRoot, dir, create); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("open feature workspace %q: %w", featureDir, err)
		}
	}
	featureRoot, err := workRoot.OpenRoot(featureDir)
	if err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("open feature workspace %q: %w", featureDir, err)
	}
	w.featureRoot = featureRoot
	// Recheck after opening so a component swap cannot silently turn the
	// persisted feature binding into a symlink-backed workspace.
	if err := ensureRootedDirectory(workRoot, featureDir, false); err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("open feature workspace %q: %w", featureDir, err)
	}
	return w, nil
}

func ensureRootedDirectory(root *os.Root, name string, create bool) error {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) && create {
		if err := root.Mkdir(name, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create directory %s: %w", name, err)
		}
		info, err = root.Lstat(name)
	}
	if err != nil {
		return fmt.Errorf("inspect directory %s: %w", name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("directory %s is a symbolic link", name)
	}
	if !info.IsDir() {
		return fmt.Errorf("directory %s is not a directory", name)
	}
	return nil
}

func (w *featureWorkspace) Close() error {
	var errs []error
	if w.featureRoot != nil {
		errs = append(errs, w.featureRoot.Close())
		w.featureRoot = nil
	}
	if w.workRoot != nil {
		errs = append(errs, w.workRoot.Close())
		w.workRoot = nil
	}
	return errors.Join(errs...)
}

func validateArtifactName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || path.Clean(name) != name || !featureNameRE.MatchString(name) {
		return fmt.Errorf("artifact name %q is malformed", name)
	}
	return nil
}

func (w *featureWorkspace) regularInfo(name string) (os.FileInfo, error) {
	if err := validateArtifactName(name); err != nil {
		return nil, err
	}
	info, err := w.featureRoot.Lstat(name)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("artifact %s/%s is a symbolic link", w.featureDir, name)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact %s/%s is not a regular file", w.featureDir, name)
	}
	return info, nil
}

func (w *featureWorkspace) readRegular(name string) ([]byte, error) {
	if _, err := w.regularInfo(name); err != nil {
		return nil, err
	}
	f, err := w.featureRoot.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("artifact %s/%s changed to a non-regular file", w.featureDir, name)
	}
	if _, err := w.regularInfo(name); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

func (w *featureWorkspace) regularSize(name string) (int64, error) {
	if _, err := w.regularInfo(name); err != nil {
		return 0, err
	}
	f, err := w.featureRoot.Open(name)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("artifact %s/%s changed to a non-regular file", w.featureDir, name)
	}
	if _, err := w.regularInfo(name); err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func preflightFeatureWorkspace(sc *StageContext) error {
	if sc.FeatureDir == "" {
		return nil
	}
	workspace, err := openFeatureWorkspace(sc.WorkDir, sc.FeatureDir, false)
	if err != nil {
		return err
	}
	defer workspace.Close()
	for _, name := range authoritativeArtifactNames {
		if _, err := workspace.regularInfo(name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("preflight feature artifact: %w", err)
		}
	}
	return nil
}

func (w *featureWorkspace) writeRegular(name string, data []byte, perm os.FileMode) error {
	if err := validateArtifactName(name); err != nil {
		return err
	}
	if _, err := w.regularInfo(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tempName, temp, err := w.createTemp(perm)
	if err != nil {
		return err
	}
	defer func() {
		_ = w.featureRoot.Remove(tempName)
	}()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write temporary artifact: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync temporary artifact: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary artifact: %w", err)
	}
	if _, err := w.regularInfo(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := w.featureRoot.Rename(tempName, name); err != nil {
		return fmt.Errorf("replace artifact %s/%s: %w", w.featureDir, name, err)
	}
	if _, err := w.regularInfo(name); err != nil {
		return err
	}
	return nil
}

func (w *featureWorkspace) createTemp(perm os.FileMode) (string, *os.File, error) {
	for i := 0; i < 10; i++ {
		suffix, err := randomSuffix(12)
		if err != nil {
			return "", nil, err
		}
		name := ".fox-autodev-" + suffix + ".tmp"
		f, err := w.featureRoot.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, fmt.Errorf("create temporary artifact: %w", err)
		}
		return name, f, nil
	}
	return "", nil, fmt.Errorf("create temporary artifact: exhausted unique names")
}
