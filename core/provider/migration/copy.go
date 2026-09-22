package provider_migration

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// CopyWorld copies every reachable block of the accepted World root and fences
// the destination. A cache inventory cannot establish completeness. Existing
// content-addressed blocks make retries safe after any interrupted write.
func CopyWorld(ctx context.Context, b bus.Bus, le *logrus.Entry, factories *block_transform.StepFactorySet, object sobject.SharedObject, accepted *sobject.SOState, destination block.StoreOps) error {
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
	ref := state.GetHeadRef()
	if ref != nil && !ref.GetRootRef().GetEmpty() {
		xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, factories, ref.GetTransformConf())
		if err != nil {
			return err
		}
		store := object.GetBlockStore()
		bucketID := store.GetID()
		localRef := ref.CloneVT()
		localRef.BucketId = bucketID
		cursor := bucket_lookup.NewCursor(ctx, b, le, factories, store, xfrm, localRef, &bucket.BucketOpArgs{BucketId: bucketID, VolumeId: bucketID}, ref.GetTransformConf())
		cursor.SetBucketIDOverride(bucketID)
		defer cursor.Release()
		worldState, err := world_block.BuildWorldStateFromCursor(ctx, le, false, cursor, world.NewWorldStorageFromCursor(cursor), nil, false)
		if err != nil {
			return err
		}
		defer worldState.Discard()
		err = worldState.WalkBlocks(ctx, func(ctx context.Context, typeID string) (block.Ctor, error) {
			lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			info, release, err := blocktype.ExLookupBlockType(lookupCtx, b, typeID)
			if release != nil {
				defer release.Release()
			}
			if err != nil {
				return nil, err
			}
			if info == nil {
				return nil, errors.Errorf("block type unavailable: %s", typeID)
			}
			return info.Constructor, nil
		}, func(ref *block.BlockRef, data []byte, refs []*block.BlockRef) error {
			_, _, err := destination.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref, Refs: refs})
			return err
		})
		if err != nil {
			return err
		}
	}
	fenced, err := destination.Sync(ctx)
	if err == nil && !fenced {
		err = errors.New("destination block store did not confirm durable storage")
	}
	return err
}
