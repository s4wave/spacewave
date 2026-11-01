package packmove

import (
	"context"

	"github.com/pkg/errors"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// walkChunk is the nodes one snapshot of the outside walk expands.
const walkChunk = 512

// OutsideWorld is the set of blocks the volume's reference graph reaches from
// a root other than the World's block store: the account's settings and every
// other store. The volume's block range holds them with the World's blocks, so
// Pack leaves them alone and Switch keeps their values.
//
// The set errs toward keeping: a block that both the World and another root
// reach is in it, and a block no root reaches is not.
type OutsideWorld struct {
	// keys are the block keys, as BlockRef.MarshalVT encodes them.
	keys map[string]struct{}
	// bytes is the size of the values of the blocks in keys the walk found.
	bytes uint64
}

// Has reports whether the block key, relative to the block prefix, is in the
// set.
func (o *OutsideWorld) Has(key []byte) bool {
	if o == nil {
		return false
	}
	_, ok := o.keys[string(key)]
	return ok
}

// Len returns the number of blocks in the set.
func (o *OutsideWorld) Len() uint64 {
	if o == nil {
		return 0
	}
	return uint64(len(o.keys))
}

// Bytes returns the size of the values of the blocks in the set.
func (o *OutsideWorld) Bytes() uint64 {
	if o == nil {
		return 0
	}
	return o.bytes
}

// outsideWalk is the state of a FindOutsideWorld walk.
type outsideWalk struct {
	// world is the graph node of the World's block store, which the walk
	// never enters.
	world string
	// seen holds the nodes already queued.
	seen map[string]struct{}
	// frontier holds the nodes not yet expanded.
	frontier []string
	// out is the set under construction.
	out *OutsideWorld
}

// queue adds the nodes in targets the walk has not met, and the blocks among
// them to the set.
func (w *outsideWalk) queue(targets []string) {
	for _, node := range targets {
		if _, ok := w.seen[node]; ok || block_gc.IsPermanentRoot(node) {
			continue
		}
		w.seen[node] = struct{}{}
		w.frontier = append(w.frontier, node)
		ref, ok := block_gc.ParseBlockIRI(node)
		if !ok {
			continue
		}
		if key, err := ref.MarshalVT(); err == nil {
			w.out.keys[string(key)] = struct{}{}
		}
	}
}

// expandNodes reads the outgoing references of each node and the size of the
// value of each block node, from tx.
func expandNodes(ctx context.Context, tx kvtx.Tx, keys *store_kvkey.KVKey, nodes []string) (targets [][]string, size uint64, err error) {
	graph := volume_kvtx.OpenRefGraph(kvtx.NewTxStore(tx))
	for _, node := range nodes {
		out, err := graph.GetOutgoingRefs(ctx, node)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "read references of %q", block_gc.FormatIRI(node))
		}
		targets = append(targets, out)
		ref, ok := block_gc.ParseBlockIRI(node)
		if !ok {
			continue
		}
		key, err := ref.MarshalVT()
		if err != nil {
			return nil, 0, err
		}
		value, _, err := tx.Get(ctx, keys.GetBlockKey(key))
		if err != nil {
			return nil, 0, errors.Wrapf(err, "read block %s", ref.MarshalString())
		}
		size += uint64(len(value))
	}
	return targets, size, nil
}

// FindOutsideWorld walks the reference graph of src from the garbage collector
// root through every node except the block store bucket named worldStoreID, and
// returns the blocks it reaches. The walk reads src in short snapshots, so it
// runs beside a writer; a block another process commits after the walk passed
// is missing from the set, which Switch avoids by walking with the source
// closed by every other process.
func FindOutsideWorld(ctx context.Context, src kvtx.Store, keys *store_kvkey.KVKey, worldStoreID string) (*OutsideWorld, error) {
	// Start from the roots' targets.
	w := &outsideWalk{
		world: block_gc.BucketIRI(worldStoreID),
		out:   &OutsideWorld{keys: make(map[string]struct{})},
	}
	w.seen = map[string]struct{}{w.world: {}}
	var roots []string
	err := readStable(ctx, src, func(tx kvtx.Tx) error {
		var err error
		roots, err = volume_kvtx.OpenRefGraph(kvtx.NewTxStore(tx)).GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "read the graph roots")
	}
	w.queue(roots)

	// Expand a chunk of nodes per snapshot until none are left.
	for len(w.frontier) != 0 {
		at := max(0, len(w.frontier)-walkChunk)
		chunk := w.frontier[at:]
		w.frontier = w.frontier[:at:at]
		var targets [][]string
		var size uint64
		err := readStable(ctx, src, func(tx kvtx.Tx) error {
			var err error
			targets, size, err = expandNodes(ctx, tx, keys, chunk)
			return err
		})
		if err != nil {
			return nil, errors.Wrap(err, "walk the graph")
		}
		w.out.bytes += size
		for _, t := range targets {
			w.queue(t)
		}
	}
	return w.out, nil
}
