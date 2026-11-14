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

// Roots returns the roots a migration of the Space object at ref copies, so
// its target replays the accepted state unchanged without the source: the
// World of the state's checkpoint and the payloads of the operations above it.
// The state is one consistent read, so an operation or checkpoint the source
// accepts later is left to the merge that follows. Operation types resolve as
// the source Space's Adapter resolves them on b, the source account's bus
// (Account.SpaceBus), so a running plugin's operations name their payloads.
// It fails with space_world.ErrUnresolvedOp while an operation's type
// resolves nowhere and the source's replay does not refuse it, such as a
// plugin's that is not running (space_world.ImportRoots).
func Roots(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	factories *block_transform.StepFactorySet,
	object sobject.SharedObject,
	ref *sobject.SharedObjectRef,
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
	return space_world.ImportRoots(ctx, le, b, factories, object, space.SpaceEngineId(ref), space_world_engine.OpTypes, snap)
}

// CopyRoots copies every block reachable from roots out of src into
// destination and fences the destination. visited, if set, observes each
// block the copy reads. A cache inventory cannot establish
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
// A store that holds no roots, such as a cloud block store, takes no stage:
// CopyRoots copies every block into it, trusting no presence check, and
// returns nil. Such a copy is safe today only because Spacewave pack reclaim
// is off by default (SetPackReclaim). A reclaim pass trashes a pack at least
// half dead by the roots a fence reports, and this copy holds nothing a fence
// reports; a trashed pack stays readable, and a pass retires it only after
// packfile.TrashAge, rescuing first the blocks that are live again.
func CopyRoots(
	ctx context.Context,
	src block.StoreOps,
	roots []*block.BlockRef,
	destination block.StoreOps,
	visited func(ref *block.BlockRef, data []byte),
) (*block.StageCopy, error) {
	// Copy every block into a store that holds no roots.
	opts := block.GraphCopyOptions{Visited: visited}
	if !block.HoldsRoots(destination) {
		for _, root := range roots {
			if err := block.CopyGraph(ctx, src, destination, root, &opts); err != nil {
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
		if err = c.Copy(ctx, src, root, opts); err != nil {
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
