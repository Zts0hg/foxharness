package collaboration

import "testing"

func TestModeValuesAreStable(t *testing.T) {
	if got := ModeDefault.String(); got != "default" {
		t.Fatalf("ModeDefault.String() = %q, want default", got)
	}
	if got := ModeFormalPlan.String(); got != "formal_plan" {
		t.Fatalf("ModeFormalPlan.String() = %q, want formal_plan", got)
	}
}

func TestModePlanEnabled(t *testing.T) {
	if ModeDefault.PlanEnabled() {
		t.Fatal("ModeDefault.PlanEnabled() = true, want false")
	}
	if !ModeFormalPlan.PlanEnabled() {
		t.Fatal("ModeFormalPlan.PlanEnabled() = false, want true")
	}
}
