//go:build !js && !tinygo

package wizard_resource

import (
	"context"
	"errors"
	"testing"

	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
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

// errPublishStep is the failure injected into one step of a clone publish.
var errPublishStep = errors.New("publish step failed")

// failingPublishEngine opens transactions that fail reads and deletes of one
// object key.
type failingPublishEngine struct {
	world.Engine
	failKey string
}

func (e *failingPublishEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &failingPublishTx{Tx: tx, failKey: e.failKey}, nil
}

type failingPublishTx struct {
	world.Tx
	failKey string
}

func (t *failingPublishTx) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	if key == t.failKey {
		return nil, false, errPublishStep
	}
	return t.Tx.GetObject(ctx, key)
}

func (t *failingPublishTx) DeleteObject(ctx context.Context, key string) (bool, error) {
	if key == t.failKey {
		return false, errPublishStep
	}
	return t.Tx.DeleteObject(ctx, key)
}

func TestPublishGitCloneCommitsTogether(t *testing.T) {
	const wizardKey, repoKey = "wizard/git/publish", "git/publish"
	cases := []struct {
		name    string
		failKey string
	}{
		{"index write fails", space_world.SpaceSettingsObjectKey},
		{"wizard delete fails", wizardKey},
		{"publish commits", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tb := world_testbed.MustDefault(t, ctx, world_testbed.WithWorldVerbose(false))
			ws := tb.WorldState

			// A wizard object that the Space index names.
			wizardObj, err := ws.CreateObject(ctx, wizardKey, nil)
			world.ReleaseObjectState(wizardObj)
			if err != nil {
				t.Fatal(err)
			}
			settings := &space_world.SpaceSettings{IndexPath: wizardKey}
			_, _, err = world.AccessWorldObject(ctx, ws, space_world.SpaceSettingsObjectKey, true, func(bcs *block.Cursor) error {
				bcs.SetBlock(settings, true)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			err = world_types.SetObjectType(ctx, ws, space_world.SpaceSettingsObjectKey, space_world.SpaceSettingsTypeID)
			if err != nil {
				t.Fatal(err)
			}
			sender, err := peer.NewPeer(nil)
			if err != nil {
				t.Fatal(err)
			}

			// Publish an empty repository through the failing engine.
			eng := &failingPublishEngine{Engine: tb.Engine, failKey: tc.failKey}
			resource := NewWizardResource(nil, eng, wizardKey, nil)
			defer resource.Close()
			err = resource.publishGitClone(ctx, repoKey, nil, nil, sender.GetPeerID())
			if tc.failKey != "" && !errors.Is(err, errPublishStep) {
				t.Fatalf("expected the injected failure, got %v", err)
			}
			if tc.failKey == "" && err != nil {
				t.Fatal(err)
			}

			// The repository, the index and the wizard all moved, or none did.
			published := tc.failKey == ""
			repoObj, repoFound, err := ws.GetObject(ctx, repoKey)
			world.ReleaseObjectState(repoObj)
			if err != nil {
				t.Fatal(err)
			}
			wizardObj, wizardFound, err := ws.GetObject(ctx, wizardKey)
			world.ReleaseObjectState(wizardObj)
			if err != nil {
				t.Fatal(err)
			}
			got, err := space_world.LookupSpaceSettingsBody(ctx, ws)
			if err != nil {
				t.Fatal(err)
			}
			wantIndex := wizardKey
			if published {
				wantIndex = repoKey
			}
			if repoFound != published || wizardFound == published || got.GetIndexPath() != wantIndex {
				t.Fatalf("repo %v, wizard %v, index %q; want published=%v", repoFound, wizardFound, got.GetIndexPath(), published)
			}
		})
	}
}
