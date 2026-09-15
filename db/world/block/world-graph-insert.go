package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// InsertGraphQuads inserts new, unique relationships in one graph index update.
// It validates all endpoints before writing and records the same endpoint
// revisions and World changes as individual SetGraphQuad calls. An existing or
// repeated relationship is an error; discard the enclosing transaction on error.
// Like other mutable WorldState operations, calls must be serialized by its owner.
func (t *WorldState) InsertGraphQuads(ctx context.Context, quads []world.GraphQuad) error {
	if !t.write {
		return tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return tx.ErrDiscarded
	}
	if err := ctx.Err(); err != nil || len(quads) == 0 {
		return err
	}

	seen := make(map[string]struct{}, len(quads))
	deltas := make([]graph.Delta, len(quads))
	endpoints := make([][2]*ObjectState, len(quads))
	for i, q := range quads {
		cq, err := world.GraphQuadToCayleyQuad(q, true)
		if err != nil {
			return err
		}
		deltas[i] = graph.Delta{Quad: cq, Action: graph.Add}
		key := cq.NQuad()
		if _, exists := seen[key]; exists {
			return &graph.DeltaError{Delta: deltas[i], Err: graph.ErrQuadExists}
		}
		seen[key] = struct{}{}
		for j, value := range [2]string{q.GetSubject(), q.GetObj()} {
			key, err := world.GraphValueToKey(value)
			if err != nil {
				return err
			}
			endpoints[i][j], err = t.mustGetObject(ctx, key)
			if err != nil {
				return err
			}
		}
	}
	if err := t.insertGraphDeltas(ctx, deltas); err != nil {
		return err
	}

	for i, q := range quads {
		for _, endpoint := range endpoints[i] {
			if _, err := endpoint.incrementRev(ctx, false); err != nil {
				return err
			}
		}
		if _, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_SET,
			Quad:       world.GraphQuadToQuad(q),
		}); err != nil {
			return err
		}
	}
	return nil
}

// insertGraphDeltas bulk-builds a fresh index instead of rewriting its pages
// for every Cayley key. Existing graphs retain their incremental update path.
func (t *WorldState) insertGraphDeltas(ctx context.Context, deltas []graph.Delta) error {
	tree, packed := t.graphTree.(*kvtx_block_okra.Tx)
	if !packed || len(deltas) < 2 {
		return t.graphHd.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{})
	}
	existing, err := t.LookupGraphQuads(ctx, world.NewGraphQuad("", "", "", ""), 1)
	if err != nil {
		return err
	}
	if len(existing) != 0 {
		return t.graphHd.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{})
	}

	index := hashmap.NewBTreeMap[[]byte]()
	opts := graph.Options{cayley_kv.OptAssumeDefaultIdx: true}
	staged, err := kvtx_cayley.NewGraph(ctx, hashmap.NewHashmapKvtx(index), opts)
	if err != nil {
		return errors.Wrap(err, "create graph import index")
	}
	defer staged.Close()
	if err := staged.ApplyDeltas(ctx, deltas, graph.IgnoreOpts{}); err != nil {
		return errors.Wrap(err, "build graph import index")
	}
	if err := tree.ReplaceAll(ctx, func(yield func([]byte, []byte) bool) {
		_ = index.Iterate(ctx, func(_ context.Context, key, value []byte) error {
			if !yield(key, value) {
				return context.Canceled
			}
			return nil
		})
	}); err != nil {
		return errors.Wrap(err, "write graph import index")
	}
	// Reopen Cayley so its cached counts and node IDs describe the new index.
	replacement, err := kvtx_cayley.NewGraph(ctx, kvtx.NewTxStore(tree), opts)
	if err != nil {
		return errors.Wrap(err, "open graph import index")
	}
	_ = t.graphHd.Close()
	t.graphHd = replacement
	return nil
}
