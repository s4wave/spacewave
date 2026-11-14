package provider_migration

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

// Checkpoint advances the accepted state of the Space object mounted at ref to
// a checkpoint, signed by the source owner, whose World is the replayed World. The receiver of the
// returned state needs only the blocks of that World, which stay held until
// release.
func Checkpoint(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	factories *block_transform.StepFactorySet,
	ref *sobject.SharedObjectRef,
	object sobject.SharedObject,
	accepted *sobject.SOState,
) (*sobject.SOState, *space_world.InnerState, func(), error) {
	host, ok := object.(sobject.InviteHost)
	if !ok {
		return nil, nil, nil, errors.New("source cannot sign its migration checkpoint")
	}
	return space_world.CheckpointWorld(ctx, le, b, factories, object, space.SpaceEngineId(ref), space_world_engine.OpTypes, host.GetPrivKey(), accepted)
}

// CopyWorld copies every reachable block of world from object into
// destination and fences the destination. A cache inventory cannot establish
// completeness, and content-addressed blocks make a retry safe after any
// interrupted write.
//
// On a store that holds roots the copy runs under a stage, which holds each
// copied block from its first put, and CopyWorld returns it. The caller keeps
// it open until a root holds the World, through
// space_world.HoldImport, then checks it with CheckHeld and releases
// it. A copy that stops early, or a process that dies, leaves only the
// stage's lease, which lapses, and a retry copies again.
//
// A store that holds no roots, such as a cloud block store, keeps every block
// it stores. CopyWorld copies every block into it, trusting no presence check,
// and returns nil.
func CopyWorld(ctx context.Context, object sobject.SharedObject, world *space_world.InnerState, destination block.StoreOps) (*block.StageCopy, error) {
	// Copy every block into a store that holds no roots.
	root := world.GetHeadRef().GetRootRef()
	if !block.HoldsRoots(destination) {
		if !root.GetEmpty() {
			if err := block.CopyGraph(ctx, object.GetBlockStore(), destination, root, nil); err != nil {
				return nil, err
			}
		}
		return nil, syncDurable(ctx, destination)
	}

	// Copy the reachable block graph into the destination under a stage.
	c, err := block.OpenStageCopy(ctx, destination, false)
	if err != nil {
		return nil, err
	}
	if !root.GetEmpty() {
		err = c.Copy(ctx, object.GetBlockStore(), root, block.GraphCopyOptions{})
	}
	if err == nil {
		err = c.Flush(ctx)
	}
	if err == nil {
		err = syncDurable(ctx, destination)
	}
	if err != nil {
		c.Release()
		return nil, err
	}
	return c, nil
}

// syncDurable syncs destination and requires its durable-storage
// confirmation.
func syncDurable(ctx context.Context, destination block.StoreOps) error {
	fenced, err := destination.Sync(ctx)
	if err == nil && !fenced {
		err = errors.New("destination block store did not confirm durable storage")
	}
	return err
}
