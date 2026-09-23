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
	if complete, err := block.RootComplete(ctx, store, head.GetRootRef()); err != nil {
		return err
	} else if complete {
		// A proof is volume-wide; a second bucket still needs root ownership.
		data, found, err := store.GetBlock(ctx, head.GetRootRef())
		if err != nil {
			return err
		}
		if !found {
			return block.ErrNotFound
		}
		if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: head.GetRootRef(), Data: data}}); err != nil {
			return err
		}
		_, err = store.Sync(ctx)
		return err
	}
	const maxPendingBytes = 4 << 20
	const proofBatchEntries = 1024
	writes := block.NewBufferedStoreWithSettings(ctx, store, &block.BufferedStoreSettings{
		MaxPendingEntries: proofBatchEntries,
		MaxPendingBytes:   maxPendingBytes,
		DrainBatchEntries: proofBatchEntries,
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
	var proofs []block.RootProof
	volumeProofs := block.SupportsRootRetention(store)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		fenced, err := writes.Sync(ctx)
		if err != nil {
			return err
		}
		if !fenced {
			return errors.New("local block store has no durability fence")
		}
		if volumeProofs {
			if err := block.MarkRootsComplete(ctx, store, proofs); err != nil {
				return err
			}
			proofs = nil
			clear(pending)
			return nil
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
		// Earlier proofs covered bytes but omitted their ownership graph.
		return "world-publication-v2/" + bucketID + "/" + domain + "/" + ref.MarshalString()
	}
	constructors := make(map[string]block.Ctor)
	err = ws.WalkBlocks(ctx, func(ctx context.Context, typeID string) (block.Ctor, error) {
		if ctor := constructors[typeID]; ctor != nil {
			return ctor, nil
		}
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		info, release, err := blocktype.ExLookupBlockType(lookupCtx, so.GetBus(), typeID)
		if release != nil {
			defer release.Release()
		}
		if err != nil {
			return nil, errors.Wrapf(err, "resolve retained World block type %q", typeID)
		}
		if info == nil {
			return nil, errors.Errorf("block type unavailable: %s", typeID)
		}
		constructors[typeID] = info.Constructor
		return info.Constructor, nil
	}, func(ref *block.BlockRef, data []byte, refs []*block.BlockRef) error {
		// Presence alone does not prove destination bucket ownership. Batch
		// every visited block, bypassing the bounded buffer for large payloads.
		var target block.StoreOps = writes
		if len(data) > maxPendingBytes {
			target = store
		}
		if err := target.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: data, Refs: refs}}); err != nil {
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
			if volumeProofs {
				return block.RootComplete(ctx, store, ref, domain)
			}
			tx, err := local.NewTransaction(ctx, false)
			if err != nil {
				return false, err
			}
			_, found, err := tx.Get(ctx, []byte(k))
			tx.Discard()
			if err != nil || !found {
				return false, err
			}
			// Collection can invalidate an older completion record. A live
			// parent still retains its descendants through the volume graph.
			return store.GetBlockExists(ctx, ref)
		},
		Complete: func(domain string, ref *block.BlockRef) error {
			pending[key(domain, ref)] = struct{}{}
			if volumeProofs {
				proofs = append(proofs, block.RootProof{Domain: domain, Ref: ref})
			}
			if len(pending) >= proofBatchEntries {
				return flush()
			}
			return nil
		},
	})
	if err != nil {
		return err
	}
	if err := flush(); err != nil {
		return err
	}
	return block.MarkRootComplete(ctx, store, head.GetRootRef())
}
