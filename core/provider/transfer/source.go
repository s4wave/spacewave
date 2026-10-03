package provider_transfer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/sirupsen/logrus"
)

// TransferSource provides read access to a provider account for transfer.
type TransferSource interface {
	// GetSharedObjectList returns the list of shared objects on the source.
	GetSharedObjectList(ctx context.Context) (*sobject.SharedObjectList, error)
	// ReplaySharedObject returns the World of a Space after its last
	// operation. Replay writes the blocks it creates to the Space's block
	// store, so it runs before the blocks are copied.
	ReplaySharedObject(ctx context.Context, le *logrus.Entry, ref *sobject.SharedObjectRef) (*sobject_world_engine.InnerState, error)
	// GetBlockStore returns the block store ops for a shared object's block store.
	GetBlockStore(ctx context.Context, ref *sobject.SharedObjectRef) (block.StoreOps, func(), error)
	// GetBlockRefs returns all block refs tracked for a shared object's block store.
	// Uses the GC ref graph to enumerate blocks belonging to the bucket.
	GetBlockRefs(ctx context.Context, ref *sobject.SharedObjectRef) ([]*block.BlockRef, error)
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
) (*sobject_world_engine.InnerState, error) {
	// Mount the shared object.
	so, relSO, err := provider.MountSharedObject(ctx, ref, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount shared object")
	}
	defer relSO()

	// Replay its World from the current snapshot.
	snapCtr, relSnap, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer relSnap()
	snap, err := snapCtr.WaitValue(ctx, nil)
	if err != nil {
		return nil, err
	}
	return sobject_world_engine.ReplayWorld(ctx, le, b, sfs, so, space.SpaceEngineId(ref), space_world_optypes.LookupWorldOp, snap)
}
