// Package collaboration defines the user-selected interaction mode shared by
// the application runtime and terminal UI.
package collaboration

// Mode identifies how the Agent collaborates on the next submitted task.
type Mode string

const (
	// ModeDefault allows the Agent to execute the user's task normally.
	ModeDefault Mode = "default"
	// ModeFormalPlan requires an explicit plan review before implementation.
	ModeFormalPlan Mode = "formal_plan"
)

// String returns the stable serialized name of the mode.
func (m Mode) String() string {
	return string(m)
}

// PlanEnabled reports whether the selected mode is Formal Plan.
func (m Mode) PlanEnabled() bool {
	return m == ModeFormalPlan
}

// Normalize maps unknown and zero values to Default.
func Normalize(m Mode) Mode {
	if m == ModeFormalPlan {
		return ModeFormalPlan
	}
	return ModeDefault
}
