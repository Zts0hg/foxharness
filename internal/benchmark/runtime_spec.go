package benchmark

import (
	"fmt"
	"slices"

	foxruntime "github.com/Zts0hg/foxharness/internal/runtime"
)

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

/* ResolveRuntimeSpec derives benchmark fidelity from the authoritative runtime profile and resolved run. */
func ResolveRuntimeSpec(runSpec foxruntime.RunSpec) (BenchmarkRuntimeSpec, error) {
	profile, err := foxruntime.ResolveProfile(foxruntime.BenchmarkEval)
	if err != nil {
		return BenchmarkRuntimeSpec{}, err
	}
	resolved, err := profile.Resolve(runSpec)
	if err != nil {
		return BenchmarkRuntimeSpec{}, err
	}
	profileSnapshot := profile.Snapshot()
	runSnapshot := resolved.Snapshot()
	if profileSnapshot.Name != foxruntime.BenchmarkEval || runSnapshot.Profile != foxruntime.BenchmarkEval {
		return BenchmarkRuntimeSpec{}, fmt.Errorf("benchmark runtime resolved unexpected profile %q", runSnapshot.Profile)
	}

	interactionPolicy := "interactive"
	if profileSnapshot.QuestionPort == "" && !profileSnapshot.PlanReview && profileSnapshot.PermissionPolicy == "none" {
		interactionPolicy = "headless"
	}
	observationPolicy := profileSnapshot.ObservationPolicy
	if observationPolicy == "structured_evaluation" {
		observationPolicy = "structured_result"
	}
	compactionPolicy := profileSnapshot.CompactionPolicy
	if compactionPolicy == "automatic" && runSnapshot.Model != "" {
		compactionPolicy = "automatic_selected_model"
	}

	return BenchmarkRuntimeSpec{
		ProviderProtocol:  runSnapshot.ProviderProtocol,
		Model:             runSnapshot.Model,
		MaxTurns:          runSnapshot.MaxTurns,
		ToolSurface:       resolved.AllowedTools(),
		PromptPolicy:      "base_project_session_memory",
		MemoryPolicy:      profileSnapshot.MemoryPolicy,
		CompactionPolicy:  compactionPolicy,
		PermissionPolicy:  profileSnapshot.PermissionPolicy,
		ObservationPolicy: observationPolicy,
		InteractionPolicy: interactionPolicy,
	}, nil
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
