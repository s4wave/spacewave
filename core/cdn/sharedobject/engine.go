package cdn_sharedobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// WorldEngine is the read-only world engine constructed for a CdnSharedObject.
// The engine supports SetRootRef for live refresh when the CDN root changes.
// A background refresh routine watches the CdnSharedObject snapshot container
// and advances Engine via SetRootRef when the published head changes.
type WorldEngine struct {
	// Engine is the read-only world block engine. The engine's own root ref
	// (via GetRootRef) is authoritative for the currently applied head.
	Engine *world_block.Engine
	// Cursor is the underlying root bucket cursor held by the engine. Release
	// via WorldEngine.Release when done; Engine itself does not own it.
	Cursor *bucket_lookup.Cursor

	// refresh runs the head-ref watcher goroutine; owned by Release.
	refresh *routine.RoutineContainer
}

// Release releases the underlying cursor and stops the refresh routine.
// Safe to call more than once; the cursor's own Release guards against
// double-release.
func (w *WorldEngine) Release() {
	// Stop head updates before releasing their backing resources.
	if w == nil {
		return
	}
	if w.refresh != nil {
		w.refresh.ClearContext()
		w.refresh = nil
	}

	// Drop the cursor reference held for this engine.
	if w.Cursor != nil {
		w.Cursor.Release()
		w.Cursor = nil
	}
}

// NewWorldEngine builds a read-only *world_block.Engine against the CDN
// SharedObject's current published head. Returns an error when the CDN Space
// has no published root yet, when decoding the head state fails, or when the
// head ref lacks a transform config. The returned engine is suitable for
// wrapping in a resource.space SpaceSharedObjectBody.
//
// The caller owns the returned WorldEngine and must call Release when done.
// Operation lookup belongs to the resource serving this engine.
//
// A background routine is started to watch the CdnSharedObject snapshot
// container and advance the engine's root ref via SetRootRef whenever the
// published head changes. The routine exits when Release is called.
func NewWorldEngine(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	so *CdnSharedObject,
) (*WorldEngine, error) {
	// Require a published head before constructing a readable world.
	inner, err := so.GetHeadInnerState()
	if err != nil {
		return nil, errors.Wrap(err, "load cdn head inner state")
	}
	if inner == nil || inner.GetHeadRef() == nil {
		if refreshErr := so.RefreshSnapshot(ctx); refreshErr != nil {
			return nil, errors.Wrap(refreshErr, "fetch cdn root pointer")
		}
		inner, err = so.GetHeadInnerState()
		if err != nil {
			return nil, errors.Wrap(err, "load cdn head inner state")
		}
		if inner == nil || inner.GetHeadRef() == nil {
			return nil, sobject.NewSharedObjectHealthError(
				sobject.NewSharedObjectLoadingHealth(
					sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				),
				errors.New("cdn shared object has no published head"),
			)
		}
	}

	// Route authored bucket references through the CDN block store.
	headRef := inner.GetHeadRef().CloneVT()
	bucketID := so.GetBlockStore().GetID()
	headRef.BucketId = bucketID

	// Decode the published head with its declared transform chain.
	sfs := transform_all.BuildFactorySet()
	transformConf := headRef.GetTransformConf()
	xfrm := block_transform.NewTransformerWithSteps(nil)
	if len(transformConf.GetSteps()) != 0 {
		xfrm, err = block_transform.NewTransformer(
			controller.ConstructOpts{Logger: le},
			sfs,
			transformConf,
		)
		if err != nil {
			return nil, errors.Wrap(err, "build transformer")
		}
	}

	// Share decoded blocks with sibling engines over this store.
	blockStore := so.GetBlockStore()
	decodedBlocks := blockStore.GetDecodedBlockCache()
	if decodedBlocks == nil {
		decodedBlocks = block.NewDecodedBlockCache()
	}

	// Hold the CDN cursor for every read and subsequent head update.
	cursor := bucket_lookup.NewCursor(
		ctx,
		b,
		le,
		sfs,
		blockStore,
		xfrm,
		headRef,
		&bucket.BucketOpArgs{
			BucketId: bucketID,
			VolumeId: bucketID,
		},
		transformConf,
	)
	cursor.SetBucketIDOverride(bucketID)
	cursor.SetDecodedBlockCache(decodedBlocks)

	// Build state access without importing application operation handlers.
	bengine, err := world_block.NewEngine(ctx, le, cursor, nil, nil, false)
	if err != nil {
		cursor.Release()
		return nil, errors.Wrap(err, "new world engine")
	}

	w := &WorldEngine{Engine: bengine, Cursor: cursor}

	// Follow published heads for the lifetime of the returned engine.
	watchable, _, _ := so.AccessSharedObjectState(ctx, nil)
	w.refresh = routine.NewRoutineContainerWithLogger(le)
	w.refresh.SetRoutine(func(rctx context.Context) error {
		return ccontainer.WatchChanges[sobject.SharedObjectStateSnapshot](
			rctx,
			nil,
			watchable,
			func(_ sobject.SharedObjectStateSnapshot) error {
				nextInner, innerErr := so.GetHeadInnerState()
				if innerErr != nil {
					le.WithError(innerErr).
						Warn("cdn engine refresh: decode head inner state failed")
					return nil
				}
				if nextInner == nil || nextInner.GetHeadRef() == nil {
					return nil
				}
				nextRef := nextInner.GetHeadRef().CloneVT()
				nextRef.BucketId = bucketID
				if setErr := bengine.SetRootRef(rctx, nextRef); setErr != nil {
					if rctx.Err() != nil {
						return rctx.Err()
					}
					le.WithError(setErr).
						Warn("cdn engine refresh: SetRootRef failed")
					return nil
				}
				return nil
			},
			nil,
		)
	})
	w.refresh.SetContext(ctx, true)
	return w, nil
}
