package app

import (
	"context"
	"reflect"
	"testing"
)

func TestInteractivePermissionStateUsesApplicationOwnedCommandsAndResults(t *testing.T) {
	want := PermissionState{
		SelectedMode: PermissionModeApprove, EffectiveMode: PermissionModeApprove,
		FullAccessRemembered: true, SessionGrantCount: 2,
	}
	controller := &contractPermissionController{state: want}
	if got := controller.PermissionState(); !reflect.DeepEqual(got, want) {
		t.Fatalf("permission state = %#v, want %#v", got, want)
	}
	if got := controller.UpdatePermissionMode(context.Background(), PermissionModeCommand{Mode: PermissionModeAsk}); got.SelectedMode != PermissionModeAsk {
		t.Fatalf("updated permission state = %#v, want Ask", got)
	}
	if got := controller.ActivateFullAccess(context.Background(), FullAccessCommand{Remember: true}); got.EffectiveMode != PermissionModeFullAccess {
		t.Fatalf("full-access state = %#v", got)
	}
	if got := controller.ClearPermissionGrants(context.Background()); got.Cleared != 2 {
		t.Fatalf("clear outcome = %#v, want two grants", got)
	}
	var _ InteractivePermissionController = controller
}

type contractPermissionController struct {
	state PermissionState
}

func (c *contractPermissionController) PermissionState() PermissionState { return c.state }

func (c *contractPermissionController) UpdatePermissionMode(_ context.Context, command PermissionModeCommand) PermissionState {
	c.state.SelectedMode = command.Mode
	c.state.EffectiveMode = command.Mode
	return c.state
}

func (c *contractPermissionController) ActivateFullAccess(_ context.Context, command FullAccessCommand) PermissionState {
	c.state.SelectedMode = PermissionModeFullAccess
	c.state.EffectiveMode = PermissionModeFullAccess
	c.state.FullAccessRemembered = command.Remember
	return c.state
}

func (c *contractPermissionController) ClearPermissionGrants(context.Context) PermissionGrantClearOutcome {
	cleared := c.state.SessionGrantCount
	c.state.SessionGrantCount = 0
	return PermissionGrantClearOutcome{Cleared: cleared, State: c.state}
}
