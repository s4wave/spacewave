package sobject_world_engine

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// retainPublicationWorld fences dependencies for asynchronously persisted providers.
func (c *Controller) retainPublicationWorld(ctx context.Context, so sobject.SharedObject, head *bucket.ObjectRef) error {
	retention, ok := so.(sobject.PublicationRetention)
	if !ok {
		return nil
	}
	if head.GetRootRef().GetEmpty() {
		return nil
	}
	local, release, err := retention.AccessPublicationRetention(ctx)
	if err != nil {
		return err
	}
	defer release()
	return RetainWorld(ctx, c.le, c.sfs, so, head, local, nil)
}

// RetainWorld copies a complete World graph into its SharedObject's block store.
// Completion proofs skip immutable subtrees only after their ownership writes
// are durable. The caller owns the proof store and serializes calls for it.
// visited observes newly traversed blocks; cached complete subtrees are omitted.
func RetainWorld(ctx context.Context, le *logrus.Entry, sfs *block_transform.StepFactorySet, so sobject.SharedObject, head *bucket.ObjectRef, local kvtx.Store, visited func(*block.BlockRef, []byte)) error {
	store := so.GetBlockStore()
	const maxPendingBytes = 4 << 20
	writes := block.NewBufferedStoreWithSettings(ctx, store, &block.BufferedStoreSettings{
		MaxPendingEntries: 128,
		MaxPendingBytes:   maxPendingBytes,
		DrainBatchEntries: 128,
	})

	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, sfs, head.GetTransformConf())
	if err != nil {
		return err
	}
	bucketID := store.GetID()
	localRef := head.CloneVT()
	localRef.BucketId = bucketID
	cursor := bucket_lookup.NewCursor(ctx, so.GetBus(), le, sfs, store, xfrm, localRef, &bucket.BucketOpArgs{BucketId: bucketID, VolumeId: bucketID}, head.GetTransformConf())
	cursor.SetBucketIDOverride(bucketID)
	defer cursor.Release()
	ws, err := world_block.BuildWorldStateFromCursor(ctx, le, false, cursor, world.NewWorldStorageFromCursor(cursor), nil, false)
	if err != nil {
		return err
	}
	defer ws.Discard()

	// Fence bytes before recording proofs. Failure may preserve complete
	// subtrees, but never records a parent whose descendants failed.
	pending := make(map[string]struct{})
	flush := func() error {
		fenced, err := writes.Sync(ctx)
		if err != nil {
			return err
		}
		if !fenced {
			return errors.New("local block store has no durability fence")
		}
		err = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
			return local.NewTransaction(ctx, true)
		}, func(ctx context.Context, tx kvtx.Tx) error {
			for key := range pending {
				if err := tx.Set(ctx, []byte(key), []byte{1}); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			clear(pending)
		}
		return err
	}
	key := func(domain string, ref *block.BlockRef) string {
		return "world-publication/" + bucketID + "/" + domain + "/" + ref.MarshalString()
	}
	err = ws.WalkBlocks(ctx, func(ctx context.Context, typeID string) (block.Ctor, error) {
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		info, release, err := blocktype.ExLookupBlockType(lookupCtx, so.GetBus(), typeID)
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
	}, func(ref *block.BlockRef, data []byte) error {
		// Presence alone does not prove destination bucket ownership. Batch
		// every visited block, bypassing the bounded buffer for large payloads.
		var target block.StoreOps = writes
		if len(data) > maxPendingBytes {
			target = store
		}
		if err := target.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data}}); err != nil {
			return err
		}
		if visited != nil {
			visited(ref, data)
		}
		return nil
	}, &world_block.WalkBlocksOptions{
		Known: func(domain string, ref *block.BlockRef) (bool, error) {
			k := key(domain, ref)
			if _, ok := pending[k]; ok {
				return true, nil
			}
			tx, err := local.NewTransaction(ctx, false)
			if err != nil {
				return false, err
			}
			defer tx.Discard()
			_, found, err := tx.Get(ctx, []byte(k))
			return found, err
		},
		Complete: func(domain string, ref *block.BlockRef) error {
			pending[key(domain, ref)] = struct{}{}
			if len(pending) >= 1024 {
				return flush()
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	return flush()
}
