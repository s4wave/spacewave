package world_block

import (
	"context"

	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/graph/proto"
	"github.com/aperturerobotics/cayley/kv/flat"
	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	kvtx_cayley_nodegraph "github.com/s4wave/spacewave/db/kvtx/cayley/nodegraph"
	hidalgo "github.com/s4wave/spacewave/db/kvtx/hidalgo"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/util/extsort"
)

// Regeneration's invariants, which every step from the pin to the release of
// the old root keeps:
//
//  1. The new World holds exactly the pinned root's object records and quads,
//     plus the changes committed since the pin, applied from the root diff.
//     The build reads only the pinned root, never the live head, since a
//     delete and a re-add after the pin cancel in the diff and would lose the
//     item a live read missed.
//  2. The old root stays held by name until a streamed comparison finds every
//     object record and quad of the two Worlds equal; a switch back to it
//     works until then.
//  3. Nothing is deleted on doubt: a failed or interrupted build deletes only
//     its own runs, and the old World keeps serving.
//  4. The build is a function of the pinned root alone: no map order, time or
//     run boundary reaches the new root, so a replica building the same root
//     writes the same blocks.

// Run bytes the sort writes per old node and quad, used to reserve disk before
// the first spill. A node writes its value map entry and its meta, each
// holding the value text, estimated at 64 bytes; a quad writes two 27-byte
// edge keys and a pin marker for its predicate, and one more for a label.
const (
	regenNodeRunBytes = 160
	regenQuadRunBytes = 96
)

// RegenConfig sets where and how much a regeneration build sorts, and when
// its catch-up is small enough to swap.
type RegenConfig struct {
	// SpillDir is the directory the sort writes its runs in. The runs are
	// disposable and removed when the build ends.
	SpillDir string
	// Budget is the sort's memory in bytes before it spills a run.
	Budget int
	// SwapChanges is the most changed log records the swap applies inside the
	// writer turn. A catch-up round that leaves more runs again off the turn,
	// so a commit waits at most for a diff of this size.
	SwapChanges int
}

// buildNodeGraphRoot writes a node graph World holding the objects, quads and
// changelog of the cayley kv World at root, and returns the new root. The
// object tree, the changelog and the sequence number carry over unchanged;
// the graph tree is rebuilt from one walk of the old graph's log. Every node
// keeps its old ID, so the build holds no map from old IDs to new.
//
// The build reads only root. Its memory is cfg.Budget plus the sort's read
// buffers, and its disk the sort's runs, reserved before it starts. It writes
// blocks through store, or bucketCursor's transaction store when store is nil,
// and publishes nothing.
func buildNodeGraphRoot(
	ctx context.Context,
	bucketCursor *bucket_lookup.Cursor,
	store block.StoreOps,
	root *block.BlockRef,
	cfg RegenConfig,
) (*block.BlockRef, error) {
	// Trace the build for the whole walk and write.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/regen/build")
	defer task.End()

	// Open the pinned root and check it is a kv World.
	transaction, worldCursor := bucketCursor.BuildTransactionAtRefWithStore(nil, root, store)
	w, err := UnmarshalWorld(ctx, worldCursor)
	if err != nil {
		return nil, errors.Wrap(err, "load pinned World")
	}
	if f := w.GetFormat(); f != WorldFormat_WORLD_FORMAT_CAYLEY_KV {
		return nil, errors.Errorf("regenerate a %v World", f)
	}

	// Sort the old graph's nodes and quads into the new key order.
	sorter := extsort.New(cfg.SpillDir, cfg.Budget)
	defer sorter.Close()
	builder := kvtx_cayley_nodegraph.NewBuilder(sorter)
	if err := addKvGraph(ctx, worldCursor, w.GetGraphSettings(), sorter, builder, int64(cfg.Budget)); err != nil {
		return nil, err
	}

	// Write the sorted keys as the new graph tree under the World.
	if err := writeBuiltGraph(ctx, worldCursor, builder); err != nil {
		return nil, errors.Wrap(err, "write node graph")
	}

	// Mark the World a node graph and write its block.
	w.Format = WorldFormat_WORLD_FORMAT_NODE_GRAPH
	w.GraphSettings = nil
	worldCursor.SetBlock(w, true)
	newRoot, _, err := transaction.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write regenerated World")
	}
	return newRoot, nil
}

// writeBuiltGraph writes builder's key stream as a new OKRA_INLINE tree and
// sets it as worldCursor's graph sub-block.
func writeBuiltGraph(ctx context.Context, worldCursor *block.Cursor, builder *kvtx_cayley_nodegraph.Builder) error {
	// Open an empty tree on a detached cursor.
	cursor := worldCursor.Detach(false)
	cursor.SetBlock(kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE), true)
	graphTx, err := kvtx_block.BuildKvTransaction(ctx, cursor, true)
	if err != nil {
		return err
	}

	// Stream the sorted keys into the tree; a stopped yield cancels the merge.
	var writeErr error
	err = graphTx.(*kvtx_block_okra.Tx).ReplaceAll(ctx, func(yield func([]byte, []byte) bool) {
		writeErr = builder.Write(ctx, func(key, value []byte) error {
			if !yield(key, value) {
				return context.Canceled
			}
			return nil
		})
	})
	if err == nil {
		err = writeErr
	}

	// Attach the tree as the World's graph sub-block.
	if err == nil {
		err = cursor.SetAsSubBlock(2, worldCursor)
	}
	if err != nil {
		graphTx.Discard()
	}
	return err
}

// addKvGraph adds every node and quad of the kv graph under worldCursor to
// builder, reserving the sort's disk first when the graph exceeds budget.
func addKvGraph(
	ctx context.Context,
	worldCursor *block.Cursor,
	settings *GraphSettings,
	sorter *extsort.Sorter,
	builder *kvtx_cayley_nodegraph.Builder,
	budget int64,
) error {
	// Open the old graph tree read-only, and its quad store for the counts.
	gtx, err := kvtx_block.BuildKvTransaction(ctx, worldCursor.FollowSubBlock(2), false)
	if err != nil {
		return errors.Wrap(err, "open kv graph tree")
	}
	defer gtx.Discard()
	handle, err := openGraph(ctx, WorldFormat_WORLD_FORMAT_CAYLEY_KV, settings, gtx)
	if err != nil {
		return errors.Wrap(err, "open kv graph")
	}
	defer handle.Close()

	// Reserve the runs' disk from the graph's counts when they would spill.
	stats, err := handle.Stats(ctx, false)
	if err != nil {
		return errors.Wrap(err, "kv graph counts")
	}
	runBytes := stats.Nodes.Value*regenNodeRunBytes + stats.Quads.Value*regenQuadRunBytes
	if runBytes > budget {
		if err := sorter.Reserve(runBytes); err != nil {
			return err
		}
	}

	// Walk the log once, through the adapter kvtx_cayley.NewGraph opens the
	// graph with; nodes keep their IDs.
	logStore := flat.Upgrade(hidalgo.NewKV(kvtx.NewTxStore(gtx)))
	return cayley_kv.WalkLog(ctx, logStore, func(p *proto.Primitive) error {
		if !p.IsNode() {
			return builder.AddQuad(p.GetSubject(), p.GetPredicate(), p.GetObject(), p.GetLabel())
		}
		v := &pquads.Value{}
		if err := v.UnmarshalVT(p.GetValue()); err != nil {
			return errors.Wrapf(err, "node %d", p.GetID())
		}
		return builder.AddNode(p.GetID(), v)
	})
}
