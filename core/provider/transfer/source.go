package provider_transfer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_engine "github.com/s4wave/spacewave/core/space/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/sirupsen/logrus"
)

// TransferSource provides read access to a provider account for transfer.
type TransferSource interface {
	// GetOrigin returns the origin naming the source account, with no head.
	GetOrigin() *TransferOrigin
	// LeavesSharedObjects reports whether deleting a shared Space from the
	// source removes only this account from it, leaving its other members
	// their Space.
	LeavesSharedObjects() bool
	// GetSharedObjectList returns the list of shared objects on the source.
	GetSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error)
	// ReplaySharedObject returns the World of a Space after its last
	// operation. Replay writes the blocks it creates to the Space's block
	// store and holds them until release, so the transfer copies the graphs
	// under the World's roots before release.
	ReplaySharedObject(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef) (*space_world.InnerState, func(), error)
	// GetBlockStore returns the block store ops for a shared object's block store.
	GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error)
}

// CleanupSource handles post-merge cleanup of the source account.
// This is separate from TransferSource to keep the read interface minimal.
type CleanupSource interface {
	// DeleteSharedObject deletes a shared object from the source account.
	DeleteSharedObject(ctx context.Context, soID string) error
	// DeleteVolume deletes the source account's storage volume.
	DeleteVolume(ctx context.Context) error
}

// replaySharedObject mounts the Space of ref on provider and replays its World.
func replaySharedObject(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	sfs *block_transform.StepFactorySet,
	provider sobject.SharedObjectProvider,
	ref *sobject.SharedObjectRef,
) (*space_world.InnerState, func(), error) {
	// Mount the shared object until release.
	so, relSO, err := provider.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, nil, errors.Wrap(err, "mount shared object")
	}
	held := false
	defer func() {
		if !held {
			relSO()
		}
	}()

	// Read the current snapshot.
	snapCtr, relSnap, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer relSnap()
	snap, err := snapCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, nil, err
	}

	// Replay its World, held with the mount until release.
	state, releaseWorld, err := space_world.ReplayWorld(ctx, le, b, sfs, so, space.SpaceEngineId(ref), space_world_engine.OpTypes, snap)
	if err != nil {
		return nil, nil, err
	}
	held = true
	return state, func() {
		releaseWorld()
		relSO()
	}, nil
}
