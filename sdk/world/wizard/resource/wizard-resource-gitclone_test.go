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
)

// errPublishStep is the failure injected into one step of a clone publish.
var errPublishStep = errors.New("publish step failed")

// failCommitKey names the commit as the failing step of a publish.
const failCommitKey = "commit"

// failingPublishEngine opens transactions that fail reads and deletes of one
// object key, or the commit when failKey is failCommitKey.
type failingPublishEngine struct {
	world.Engine
	failKey string
}

// NewTransaction wraps the engine's transaction to inject the failure.
func (e *failingPublishEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &failingPublishTx{Tx: tx, failKey: e.failKey}, nil
}

// failingPublishTx is a transaction that fails one step of a publish.
type failingPublishTx struct {
	world.Tx
	failKey string
}

// GetObject fails a read of the failing key.
func (t *failingPublishTx) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	if key == t.failKey {
		return nil, false, errPublishStep
	}
	return t.Tx.GetObject(ctx, key)
}

// DeleteObject fails a delete of the failing key.
func (t *failingPublishTx) DeleteObject(ctx context.Context, key string) (bool, error) {
	if key == t.failKey {
		return false, errPublishStep
	}
	return t.Tx.DeleteObject(ctx, key)
}

// Commit fails when the commit is the failing step.
func (t *failingPublishTx) Commit(ctx context.Context) error {
	if t.failKey == failCommitKey {
		return errPublishStep
	}
	return t.Tx.Commit(ctx)
}

// TestPublishGitCloneCommitsTogether checks that a clone publish creates the
// repository, moves the Space index and deletes the wizard together, and that
// a failure or a refusal at any step leaves all three as they were.
func TestPublishGitCloneCommitsTogether(t *testing.T) {
	const wizardKey, repoKey = "wizard/git/publish", "git/publish"
	cases := []struct {
		name string
		// failKey names the injected failure, or "" for none.
		failKey string
		// wizardGone skips creating the wizard.
		wizardGone bool
		// repoTaken creates an object at the repository key first.
		repoTaken bool
		// subIndex points the Space index at a path under the wizard.
		subIndex bool
		// wantErr reports whether the publish error is the expected one.
		wantErr func(error) bool
	}{
		{name: "settings read fails", failKey: space_world.SpaceSettingsObjectKey, wantErr: isPublishStep},
		{name: "wizard delete fails", failKey: wizardKey, wantErr: isPublishStep},
		{name: "commit fails", failKey: failCommitKey, wantErr: isPublishStep},
		{name: "wizard already deleted", wizardGone: true, wantErr: func(err error) bool {
			return errors.Is(err, ErrWizardGone)
		}},
		{name: "repository key taken", repoTaken: true, wantErr: func(err error) bool {
			var taken *GitRepoKeyTakenError
			return errors.As(err, &taken) && taken.Key == repoKey
		}},
		{name: "publish commits", wantErr: isNil},
		{name: "index under the wizard stays", subIndex: true, wantErr: isNil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			tb := world_testbed.MustDefault(t, ctx, world_testbed.WithWorldVerbose(false))
			ws := tb.WorldState

			// A wizard object that the Space index names, unless it is gone.
			mustCreate := func(key string) {
				obj, err := ws.CreateObject(ctx, key, nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					t.Fatal(err)
				}
			}
			if !tc.wizardGone {
				mustCreate(wizardKey)
			}
			if tc.repoTaken {
				mustCreate(repoKey)
			}
			index := wizardKey
			if tc.subIndex {
				index = wizardKey + "/sub"
			}
			settings := &space_world.SpaceSettings{IndexPath: index}
			_, _, err := world.AccessWorldObject(ctx, ws, space_world.SpaceSettingsObjectKey, true, func(bcs *block.Cursor) error {
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
			if !tc.wantErr(err) {
				t.Fatalf("unexpected publish result: %v", err)
			}

			// The repository, the index and the wizard all moved, or none did; an
			// index that names a path under the wizard never moves.
			published := err == nil
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
			wantRepo, wantWizard, wantIndex := tc.repoTaken, !tc.wizardGone, index
			if published {
				wantRepo, wantWizard = true, false
				if !tc.subIndex {
					wantIndex = repoKey
				}
			}
			if repoFound != wantRepo || wizardFound != wantWizard || got.GetIndexPath() != wantIndex {
				t.Fatalf("repo %v, wizard %v, index %q; want repo %v, wizard %v, index %q",
					repoFound, wizardFound, got.GetIndexPath(), wantRepo, wantWizard, wantIndex)
			}
		})
	}
}

// isPublishStep reports whether err is the injected publish failure.
func isPublishStep(err error) bool {
	return errors.Is(err, errPublishStep)
}

// isNil reports whether the publish succeeded.
func isNil(err error) bool {
	return err == nil
}
