package provider_migration

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

// Checkpoint advances the accepted state of the Space object mounted at ref to
// a checkpoint, signed by the source owner, whose World is the replayed World. The receiver of the
// returned state needs only the blocks of that World.
func Checkpoint(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	factories *block_transform.StepFactorySet,
	ref *sobject.SharedObjectRef,
	object sobject.SharedObject,
	accepted *sobject.SOState,
) (*sobject.SOState, *sobject_world_engine.InnerState, error) {
	host, ok := object.(sobject.InviteHost)
	if !ok {
		return nil, nil, errors.New("source cannot sign its migration checkpoint")
	}
	return sobject_world_engine.CheckpointWorld(ctx, le, b, factories, object, space.SpaceEngineId(ref), space_world_optypes.LookupWorldOp, host.GetPrivKey(), accepted)
}

// CopyWorld copies every reachable block of world from object into
// destination and fences the destination. A cache inventory cannot establish
// completeness. Existing content-addressed blocks make retries safe after any
// interrupted write.
func CopyWorld(ctx context.Context, object sobject.SharedObject, world *sobject_world_engine.InnerState, destination block.StoreOps) error {
	// Copy the reachable block graph into the destination store.
	head := world.GetHeadRef()
	if head != nil && !head.GetRootRef().GetEmpty() {
		if err := block.CopyGraph(ctx, object.GetBlockStore(), destination, head.GetRootRef(), nil); err != nil {
			return err
		}
	}

	// Sync the destination and require its durable-storage confirmation.
	fenced, err := destination.Sync(ctx)
	if err == nil && !fenced {
		err = errors.New("destination block store did not confirm durable storage")
	}
	return err
}
