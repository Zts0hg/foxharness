package benchmark

import "slices"

// BenchmarkRuntimeSpec is the immutable resolved construction input for one benchmark runtime.
type BenchmarkRuntimeSpec struct {
	ProviderProtocol  string   `json:"provider_protocol"`
	Model             string   `json:"model"`
	MaxTurns          int      `json:"max_turns"`
	ToolSurface       []string `json:"tool_surface"`
	PromptPolicy      string   `json:"prompt_policy"`
	MemoryPolicy      string   `json:"memory_policy"`
	CompactionPolicy  string   `json:"compaction_policy"`
	PermissionPolicy  string   `json:"permission_policy"`
	ObservationPolicy string   `json:"observation_policy"`
	InteractionPolicy string   `json:"interaction_policy"`
}

// NewRuntimeSpec resolves the current benchmark-only runtime construction DTO.
func NewRuntimeSpec(protocol, model string, maxTurns int, tools []string) BenchmarkRuntimeSpec {
	return BenchmarkRuntimeSpec{
		ProviderProtocol:  protocol,
		Model:             model,
		MaxTurns:          maxTurns,
		ToolSurface:       slices.Clone(tools),
		PromptPolicy:      "base_project_session_memory",
		MemoryPolicy:      "session_only",
		CompactionPolicy:  "automatic_selected_model",
		PermissionPolicy:  "none",
		ObservationPolicy: "structured_result",
		InteractionPolicy: "headless",
	}
}

// Fidelity derives machine-readable and human-readable fidelity from one spec.
func (s BenchmarkRuntimeSpec) Fidelity() RuntimeFidelity {
	snapshot := s
	snapshot.ToolSurface = slices.Clone(s.ToolSurface)
	shared := []string{"structured tool failure semantics"}
	if slices.Contains(s.ToolSurface, "read_todo") && slices.Contains(s.ToolSurface, "update_todo") {
		shared = append(shared, "todo tool surface")
	}
	if s.CompactionPolicy == "automatic_selected_model" {
		shared = append(shared, "context compaction")
	}
	differences := []string{}
	if s.PermissionPolicy == "none" {
		differences = append(differences, "no interactive approval surface")
	}
	if s.InteractionPolicy == "headless" {
		differences = append(differences, "no TUI ask_user_question surface")
	}
	return RuntimeFidelity{
		Spec:                   snapshot,
		SharedInvariants:       shared,
		IntentionalDifferences: differences,
		Warning:                "benchmark runtime intentionally reports product-runtime differences",
	}
}
