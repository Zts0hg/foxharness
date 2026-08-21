package architecturetest

import (
	"path/filepath"
	"testing"
)

func TestTUIInteractionsUseApplicationContracts(t *testing.T) {
	root := moduleRoot(t)
	files := map[string][]string{
		"internal/tui/approval_form.go":     {"internal/permission", "internal/toolpolicy"},
		"internal/tui/asker.go":             {"internal/tools"},
		"internal/tui/askform.go":           {"internal/tools"},
		"internal/tui/model.go":             {"internal/permission", "internal/toolpolicy"},
		"internal/tui/permission_bridge.go": {"internal/permission"},
		"internal/tui/permission_form.go":   {"internal/permission"},
		"internal/tui/plan_reviewer.go":     {"internal/tools"},
		"internal/tui/planform.go":          {"internal/tools"},
		"internal/tui/snapshot.go":          {"internal/permission"},
		"internal/tui/view.go":              {"internal/permission"},
	}
	for name, forbidden := range files {
		imports := fileImports(t, filepath.Join(root, name))
		if !imports["internal/app"] {
			t.Errorf("%s must import internal/app", name)
		}
		for _, dependency := range forbidden {
			if imports[dependency] {
				t.Errorf("%s retains concrete interaction dependency %s", name, dependency)
			}
		}
	}
}
