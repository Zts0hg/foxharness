package subagent

import (
	"testing"
)

func TestManager_BuildRegistry_AllowedToolsFilters(t *testing.T) {
	m := NewManager(nil, t.TempDir())

	// Without restrictions, read+bash+write+edit are all available.
	full := m.buildRegistry(false, nil)
	if got := len(full.GetAvailableTools()); got != 4 {
		t.Errorf("full registry tools = %d, want 4", got)
	}

	// With an allow-list, only the named tools survive — write_file and
	// bash that would have been registered are filtered out.
	restricted := m.buildRegistry(false, []string{"read_file"})
	defs := restricted.GetAvailableTools()
	if len(defs) != 1 || defs[0].Name != "read_file" {
		t.Errorf("restricted tools = %v, want [read_file]", defs)
	}
}

func TestManager_BuildRegistry_ReadOnlyPlusAllowedTools(t *testing.T) {
	// readOnly already drops write/edit. Adding allow-list further
	// constrains the survivors — the intersection is the right
	// semantic for skills that want both safety nets.
	m := NewManager(nil, t.TempDir())
	got := m.buildRegistry(true, []string{"read_file", "bash"})
	defs := got.GetAvailableTools()
	names := map[string]bool{}
	for _, d := range defs {
		names[d.Name] = true
	}
	if !names["read_file"] || !names["bash"] {
		t.Errorf("intersection should include read_file and bash, got %v", names)
	}
	if names["write_file"] || names["edit_file"] {
		t.Errorf("write/edit must be excluded (read-only): %v", names)
	}
}

func TestManager_BuildRegistry_AllowedToolsExcludesEverything(t *testing.T) {
	// Tools in the allow-list that the manager doesn't register are
	// silently dropped; tools the manager registers that aren't in
	// the allow-list are filtered out. Net: zero exposed tools, but
	// the registry remains a valid tools.Registry — degraded, not
	// broken.
	m := NewManager(nil, t.TempDir())
	got := m.buildRegistry(false, []string{"todo_read"}) // not registered
	if n := len(got.GetAvailableTools()); n != 0 {
		t.Errorf("expected 0 surviving tools, got %d", n)
	}
}
