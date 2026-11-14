//go:build !js

package wizard_resource

import (
	"context"
	"errors"
	"testing"

	wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

func TestWizardResourceClosePreventsLateStatePublish(t *testing.T) {
	// Close a wizard resource before a late state update arrives.
	resource := NewWizardResource(nil, nil, "", &wizard.WizardState{Name: "Initial"})
	resource.Close()

	// Deliver the wizard update after resource shutdown.
	resource.setWizardStateWatchState(&wizard.WizardState{Name: "Late"}, 1)

	// Verify the closed wizard still reports cancellation.
	var snap *wizardStateWatchSnapshot
	resource.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snap = resource.snapshotWizardStateWatchLocked()
	})
	if !errors.Is(snap.err, context.Canceled) {
		t.Fatalf("expected context.Canceled after late state publish, got %v", snap.err)
	}
}

func TestWizardResourceRejectsStaleWorldStatePublish(t *testing.T) {
	// Create a wizard resource with its initial state.
	resource := NewWizardResource(nil, nil, "", &wizard.WizardState{Name: "Initial"})

	// Deliver a current wizard revision followed by a stale revision.
	resource.setWizardStateWatchState(&wizard.WizardState{Name: "Current"}, 2)
	resource.setWizardStateWatchState(&wizard.WizardState{Name: "Stale"}, 1)

	// Verify the wizard retains the newest revision.
	var snap *wizardStateWatchSnapshot
	resource.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		snap = resource.snapshotWizardStateWatchLocked()
	})
	if snap.state.GetName() != "Current" {
		t.Fatalf("expected stale state to be ignored, got %q", snap.state.GetName())
	}
}
