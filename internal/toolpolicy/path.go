package toolpolicy

import (
	"os"
	"path/filepath"
	"strings"
)

// PathScope resolves symlinks through the nearest existing ancestor and
// classifies target relative to workspace.
func PathScope(workspace, cwd, target string) (Scope, bool) {
	if strings.TrimSpace(workspace) == "" || strings.TrimSpace(target) == "" {
		return ScopeUnknown, false
	}
	workspace = cleanPath(workspace)
	if strings.TrimSpace(cwd) == "" {
		cwd = workspace
	}
	var full string
	if filepath.IsAbs(target) {
		full = cleanPath(target)
	} else {
		full = cleanPath(filepath.Join(cwd, target))
	}
	resolvedTarget, ok := resolvePath(full)
	if !ok {
		return ScopeUnknown, false
	}
	resolvedWorkspace, ok := resolvePath(workspace)
	if !ok {
		return ScopeUnknown, false
	}
	rel, err := filepath.Rel(resolvedWorkspace, resolvedTarget)
	if err != nil {
		return ScopeUnknown, false
	}
	if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
		return ScopeWorkspace, true
	}
	return ScopeExternal, true
}

func resolvePath(path string) (string, bool) {
	path = cleanPath(path)
	existing := path
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			return "", false
		}
		suffix = append([]string{filepath.Base(existing)}, suffix...)
		existing = parent
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", false
	}
	parts := append([]string{resolved}, suffix...)
	return cleanPath(filepath.Join(parts...)), true
}

func cleanPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}
