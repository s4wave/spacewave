package provider_migration

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/sirupsen/logrus"
)

// CopyWorld copies every reachable block of the accepted World root and fences
// the destination. A cache inventory cannot establish completeness. Existing
// content-addressed blocks make retries safe after any interrupted write.
func CopyWorld(ctx context.Context, le *logrus.Entry, factories *block_transform.StepFactorySet, object sobject.SharedObject, accepted *sobject.SOState, destination block.StoreOps) error {
	host, ok := object.(sobject.InviteHost)
	if !ok {
		return errors.New("source cannot decrypt its accepted migration checkpoint")
	}
	snapshot := sobject.NewSOStateParticipantHandle(le, factories, object.GetSharedObjectID(), accepted, host.GetPrivKey(), object.GetPeerID())
	root, err := snapshot.GetRootInner(ctx)
	if err != nil {
		return err
	}
	state := &sobject_world_engine.InnerState{}
	if err := state.UnmarshalVT(root.GetStateData()); err != nil {
		return err
	}
	head := state.GetHeadRef()
	if head != nil && !head.GetRootRef().GetEmpty() {
		if err := block.CopyGraph(ctx, object.GetBlockStore(), destination, head.GetRootRef(), nil); err != nil {
			return err
		}
	}
	fenced, err := destination.Sync(ctx)
	if err == nil && !fenced {
		err = errors.New("destination block store did not confirm durable storage")
	}
	return err
}
