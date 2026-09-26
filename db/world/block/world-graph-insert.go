package world_block

import (
	"context"
	"slices"

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

// graphImportBatchSize is the number of deltas a fresh graph index import
// applies per Cayley call.
const graphImportBatchSize = 8192

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
	// Keep first-seen order while counting both ends of every relationship.
	// A shared endpoint needs one lookup and one revision mutation per batch.
	type endpointUpdate struct {
		state *ObjectState
		count uint64
	}
	endpointIndexes := make(map[string]int)
	var endpoints []endpointUpdate
	for i, q := range quads {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		for _, value := range [2]string{q.GetSubject(), q.GetObj()} {
			key, err := world.GraphValueToKey(value)
			if err != nil {
				return err
			}
			if index, ok := endpointIndexes[key]; ok {
				endpoints[index].count++
				continue
			}
			state, err := t.mustGetObject(ctx, key)
			if err != nil {
				return err
			}
			endpointIndexes[key] = len(endpoints)
			endpoints = append(endpoints, endpointUpdate{state: state, count: 1})
		}
	}
	if err := t.insertGraphDeltas(ctx, deltas); err != nil {
		return err
	}

	for _, endpoint := range endpoints {
		if _, err := endpoint.state.incrementRevBy(ctx, endpoint.count, false); err != nil {
			return err
		}
	}
	for _, q := range quads {
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
	_, packed := t.graphTree.(*kvtx_block_okra.Tx)
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
	// Bounded batches keep Cayley's per-call delta indexes small. Each batch
	// resolves the nodes written by the batches before it.
	for batch := range slices.Chunk(deltas, graphImportBatchSize) {
		if err := staged.ApplyDeltas(ctx, batch, graph.IgnoreOpts{}); err != nil {
			return errors.Wrap(err, "build graph import index")
		}
	}
	if err := t.packSnapshotGraph(ctx, index); err != nil {
		return errors.Wrap(err, "write graph import index")
	}
	// Reopen Cayley so its cached counts and node IDs describe the new index.
	replacement, err := kvtx_cayley.NewGraph(ctx, kvtx.NewTxStore(t.graphTree), opts)
	if err != nil {
		return errors.Wrap(err, "open graph import index")
	}
	_ = t.graphHd.Close()
	t.graphHd = replacement
	return nil
}
