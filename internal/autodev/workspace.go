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
	workRoot     *os.Root
	codexRoot    *os.Root
	specsRoot    *os.Root
	featureRoot  *os.Root
	featureDir   string
	openArtifact func(string) (*os.File, error)
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
	featureName := strings.TrimPrefix(featureDir, ".codexspec/specs/")
	steps := []struct {
		parent **os.Root
		name   string
		target **os.Root
	}{
		{parent: &w.workRoot, name: ".codexspec", target: &w.codexRoot},
		{parent: &w.codexRoot, name: "specs", target: &w.specsRoot},
		{parent: &w.specsRoot, name: featureName, target: &w.featureRoot},
	}
	for _, step := range steps {
		if err := ensureRootedDirectory(*step.parent, step.name, create); err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("open feature workspace %q: %w", featureDir, err)
		}
		opened, err := openVerifiedRoot(*step.parent, step.name, nil)
		if err != nil {
			_ = w.Close()
			return nil, fmt.Errorf("open feature workspace %q: %w", featureDir, err)
		}
		*step.target = opened
	}
	w.openArtifact = w.featureRoot.Open
	return w, nil
}

func openVerifiedRoot(parent *os.Root, name string, afterOpen func() error) (*os.Root, error) {
	expected, err := parent.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("inspect directory %s: %w", name, err)
	}
	opened, err := parent.OpenRoot(name)
	if err != nil {
		return nil, fmt.Errorf("open directory %s: %w", name, err)
	}
	if afterOpen != nil {
		if err := afterOpen(); err != nil {
			_ = opened.Close()
			return nil, err
		}
	}
	openedInfo, openErr := opened.Stat(".")
	current, currentErr := parent.Lstat(name)
	if openErr != nil || currentErr != nil || !os.SameFile(expected, openedInfo) || !os.SameFile(openedInfo, current) {
		_ = opened.Close()
		return nil, fmt.Errorf("directory %s changed while opening: %w", name, errors.Join(openErr, currentErr))
	}
	return opened, nil
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
	if w.specsRoot != nil {
		errs = append(errs, w.specsRoot.Close())
		w.specsRoot = nil
	}
	if w.codexRoot != nil {
		errs = append(errs, w.codexRoot.Close())
		w.codexRoot = nil
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
	expected, err := w.regularInfo(name)
	if err != nil {
		return nil, err
	}
	f, err := w.openArtifact(name)
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
	current, err := w.regularInfo(name)
	if err != nil {
		return nil, err
	}
	if !os.SameFile(expected, info) || !os.SameFile(info, current) {
		return nil, fmt.Errorf("artifact %s/%s changed while opening", w.featureDir, name)
	}
	return io.ReadAll(f)
}

func (w *featureWorkspace) regularSize(name string) (int64, error) {
	expected, err := w.regularInfo(name)
	if err != nil {
		return 0, err
	}
	f, err := w.openArtifact(name)
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
	current, err := w.regularInfo(name)
	if err != nil {
		return 0, err
	}
	if !os.SameFile(expected, info) || !os.SameFile(info, current) {
		return 0, fmt.Errorf("artifact %s/%s changed while opening", w.featureDir, name)
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
