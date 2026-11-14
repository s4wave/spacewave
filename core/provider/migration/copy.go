package provider_migration

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/sirupsen/logrus"
)

// Roots returns the roots a migration of the Space object copies, so its
// target replays the accepted state unchanged without the source: the World
// of the state's checkpoint and the payloads of the operations above it. The
// state is one consistent read, so an operation or checkpoint the source
// accepts later is left to the merge that follows. It fails with
// space_world.ErrUnresolvedOp while an operation's type does not resolve.
func Roots(
	ctx context.Context,
	le *logrus.Entry,
	factories *block_transform.StepFactorySet,
	object sobject.SharedObject,
	accepted *sobject.SOState,
) ([]*block.BlockRef, error) {
	// Read the state as the source's participant.
	host, ok := object.(sobject.InviteHost)
	if !ok {
		return nil, errors.New("source cannot read its migration state")
	}
	soHost := host.GetSOHost()
	snap := sobject.NewSOStateParticipantHandle(le, factories, object.GetSharedObjectID(), accepted, host.GetPrivKey(), object.GetPeerID()).
		WithConfigHistory(soHost.ReadConfigEntry).
		WithOperationVerifier(soHost.GetOperationVerifier()).
		WithGrantKeyring(soHost.GetGrantKeyring())
	return space_world.ImportRoots(ctx, snap, space_world_optypes.LookupWorldOp)
}

// CopyRoots copies every block reachable from roots out of object into
// destination and fences the destination. A cache inventory cannot establish
// completeness, and content-addressed blocks make a retry safe after any
// interrupted write.
//
// On a store that holds roots the copy runs under a stage, which holds each
// copied block from its first put, and CopyRoots returns it. The caller keeps
// it open until a root holds every one of roots, through
// space_world.HoldImport, then checks them with CheckHeld and releases it. A
// copy that stops early, or a process that dies, leaves only the stage's
// lease, which lapses, and a retry copies again.
//
// A store that holds no roots, such as a cloud block store, keeps every block
// it stores. CopyRoots copies every block into it, trusting no presence check,
// and returns nil.
func CopyRoots(ctx context.Context, object sobject.SharedObject, roots []*block.BlockRef, destination block.StoreOps) (*block.StageCopy, error) {
	// Copy every block into a store that holds no roots.
	src := object.GetBlockStore()
	if !block.HoldsRoots(destination) {
		for _, root := range roots {
			if err := block.CopyGraph(ctx, src, destination, root, nil); err != nil {
				return nil, err
			}
		}
		return nil, syncDurable(ctx, destination)
	}

	// Copy the reachable block graphs into the destination under a stage.
	c, err := block.OpenStageCopy(ctx, destination, false)
	if err != nil {
		return nil, err
	}
	for _, root := range roots {
		if err = c.Copy(ctx, src, root, block.GraphCopyOptions{}); err != nil {
			break
		}
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
