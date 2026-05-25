package slash

import "github.com/Zts0hg/foxharness/internal/tools"

// NewFilteredRegistry is a thin shim retained so existing slash callers
// keep compiling. The implementation lives in internal/tools because the
// concept (a tool-registry decorator with an allow-list) belongs to that
// layer and is now used by both the slash TUI path and the subagent
// fork-mode path. Prefer the tools-level constructor in new code.
func NewFilteredRegistry(base tools.Registry, allowed []string) tools.Registry {
	return tools.NewFilteredRegistry(base, allowed)
}
