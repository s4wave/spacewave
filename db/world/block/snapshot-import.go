package world_block

import (
	"context"
	"iter"
	"maps"
	"slices"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
)

// graphImportBatchSize is the number of relationships an import applies to
// its staged graph index per Cayley call.
const graphImportBatchSize = 8192

// ImportSnapshot writes a new World from its complete contents and syncs it
// into storage. Objects must arrive in strictly increasing key order; each body
// becomes the root block of its object in the storage bucket. Relationships
// must be unique and join objects of the import. Revisions and the World
// sequence match creating the same objects and relationships with ordinary
// mutations. Individual change records are omitted.
//
// Every block of an import is reachable from its root, so blocks drain into
// storage as the write buffer fills and each object body is released once
// written. A failed import can leave unpublished blocks in storage. The caller
// publishes the returned reference in its enclosing World transaction. No
// reference is returned on failure.
func ImportSnapshot(
	ctx context.Context,
	storage world.WorldStorage,
	objects iter.Seq2[string, block.Block],
	quads []world.GraphQuad,
) (*bucket.ObjectRef, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/import-snapshot")
	defer task.End()

	var result *bucket.ObjectRef
	err := storage.AccessWorldState(ctx, nil, func(bucketCursor *bucket_lookup.Cursor) error {
		// Drain with the bounds of ordinary World writes. Sync fences the
		// remainder through the destination's normal durability path.
		writes := block.NewBufferedStore(ctx, bucketCursor.GetBucket())
		bucketCursor.SetTransactionStore(writes)
		root, err := importWorld(ctx, bucketCursor, objects, quads)
		if err != nil {
			return err
		}
		if _, err := writes.Sync(ctx); err != nil {
			return errors.Wrap(err, "sync snapshot")
		}
		result = bucketCursor.GetRefWithOpArgs()
		result.RootRef = root
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// importWorld builds the World blocks of an import and returns its root. The
// object index is built from the object stream in one pass, so no mutable
// index is staged.
func importWorld(
	ctx context.Context,
	bucketCursor *bucket_lookup.Cursor,
	objects iter.Seq2[string, block.Block],
	quads []world.GraphQuad,
) (*block.BlockRef, error) {
	// Validate every relationship and count the ends of each object before
	// any object is written, since an object's revision includes them.
	for _, q := range quads {
		if _, err := world.GraphQuadToCayleyQuad(q, true); err != nil {
			return nil, err
		}
	}
	if err := requireUniqueGraphQuads(quads); err != nil {
		return nil, err
	}
	ends := make(map[string]uint64)
	for _, q := range quads {
		for _, value := range [2]string{q.GetSubject(), q.GetObj()} {
			key, err := world.GraphValueToKey(value)
			if err != nil {
				return nil, err
			}
			ends[key]++
		}
	}

	// Build the object index from the stream. Each body is written by its own
	// transaction and referenced within the World, as CreateObject does for
	// same-bucket bodies.
	transaction, worldCursor := bucketCursor.BuildTransactionAtRef(nil, nil)
	worldCursor.SetBlock(NewWorld(true), true)
	objectCursor := worldCursor.Detach(false)
	objectCursor.SetBlock(kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA), true)
	objectTx, err := kvtx_block.BuildKvTransaction(ctx, objectCursor, true)
	if err != nil {
		return nil, err
	}
	objectTree := objectTx.(*kvtx_block_okra.Tx)
	bodyRef := bucketCursor.GetRefWithOpArgs()
	bodyRef.BucketId = ""
	var count uint64
	var bodyErr error
	err = objectTree.ReplaceAllBlocks(ctx, func(yield func([]byte, block.Block) bool) {
		for key, body := range objects {
			btx, cursor := bucketCursor.BuildTransactionAtRef(nil, nil)
			cursor.SetBlock(body, true)
			ref := bodyRef.CloneVT()
			ref.RootRef, _, bodyErr = btx.Write(ctx, true)
			if bodyErr != nil {
				bodyErr = errors.Wrapf(bodyErr, "write object %s", key)
				return
			}
			obj := NewObject(key, ref)
			obj.Rev += ends[key]
			delete(ends, key)
			count++
			if !yield([]byte(objectKeyPrefix+key), obj) {
				return
			}
		}
	})
	if err == nil {
		err = bodyErr
	}
	if err == nil {
		err = objectCursor.SetAsSubBlock(1, worldCursor)
	}
	if err != nil {
		objectTx.Discard()
		return nil, err
	}
	if len(ends) != 0 {
		return nil, errors.Wrapf(world.ErrObjectNotFound, "relationship endpoint %s", slices.Sorted(maps.Keys(ends))[0])
	}

	// Build the graph index once all objects exist, then record one change per
	// object and relationship.
	index, err := buildGraphIndex(ctx, quads)
	if err != nil {
		return nil, err
	}
	if err := writeGraphIndex(ctx, worldCursor, index); err != nil {
		return nil, errors.Wrap(err, "write graph index")
	}
	worldCursor.FollowSubBlock(3).SetBlock(&ChangeLogLL{Seqno: count + uint64(len(quads))}, true)
	root, _, err := transaction.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write snapshot")
	}
	return root, nil
}

// buildGraphIndex applies unique relationships to an in-memory Cayley index.
// Bounded batches keep Cayley's per-call delta indexes small; each batch
// resolves the nodes written by the batches before it.
func buildGraphIndex(ctx context.Context, quads []world.GraphQuad) (*hashmap.BTreeMap[[]byte], error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/import-snapshot/build-graph")
	defer task.End()

	index := hashmap.NewBTreeMap[[]byte]()
	staged, err := kvtx_cayley.NewGraph(ctx, hashmap.NewHashmapKvtx(index), graph.Options{cayley_kv.OptAssumeDefaultIdx: true})
	if err != nil {
		return nil, errors.Wrap(err, "create graph index")
	}
	defer staged.Close()
	for batch := range slices.Chunk(quads, graphImportBatchSize) {
		deltas := make([]graph.Delta, len(batch))
		for i, q := range batch {
			cq, err := world.GraphQuadToCayleyQuad(q, false)
			if err != nil {
				return nil, err
			}
			deltas[i] = graph.Delta{Quad: cq, Action: graph.Add}
		}
		if err := staged.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{}); err != nil {
			return nil, errors.Wrap(err, "build graph index")
		}
	}
	return index, nil
}

// writeGraphIndex packs a built graph index into the World's graph sub-block.
func writeGraphIndex(ctx context.Context, worldCursor *block.Cursor, index *hashmap.BTreeMap[[]byte]) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/import-snapshot/write-graph")
	defer task.End()

	cursor := worldCursor.Detach(false)
	cursor.SetBlock(kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE), true)
	graphTx, err := kvtx_block.BuildKvTransaction(ctx, cursor, true)
	if err != nil {
		return err
	}
	err = graphTx.(*kvtx_block_okra.Tx).ReplaceAll(ctx, func(yield func([]byte, []byte) bool) {
		// Iterate fails only when yield stops it or ctx ends, and ReplaceAll
		// reports both.
		_ = index.Iterate(ctx, func(_ context.Context, key, value []byte) error {
			if !yield(key, value) {
				return context.Canceled
			}
			return nil
		})
	})
	if err == nil {
		err = cursor.SetAsSubBlock(2, worldCursor)
	}
	if err != nil {
		graphTx.Discard()
	}
	return err
}
