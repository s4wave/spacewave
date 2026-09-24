// Package block_gc implements garbage collection for Hydra block stores.
//
// The ref graph tracks reference edges using a single gc/ref predicate
// stored in a KVtx store. Nodes that lose all incoming gc/ref edges are
// marked unreferenced. The Collector sweeps unreferenced nodes, removing
// their outgoing edges (which may cascade further orphans), calling the
// onSwept callback, and physically deleting block-backed nodes via the
// underlying store.
//
// Content-addressed blocks cannot have reference cycles (hash depends on
// content which includes refs), so reference counting is sufficient.
package block_gc

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ErrAtomicSweepUnsupported rejects a sweep outside the store's atomic
// ownership domain before graph or block mutation.
var ErrAtomicSweepUnsupported = errors.New("atomic sweep unsupported")

// AtomicSweepStore rechecks current ownership and removes still-orphaned nodes
// and their physical blocks in one transaction, serialized with publication. A
// candidate snapshot alone never authorizes deletion. The graph argument binds
// this operation to the collector's reachability scope. It returns the nodes it
// removed; a failed transaction removes none.
type AtomicSweepStore interface {
	SweepUnreferenced(ctx context.Context, graph RefGraphOps, nodes []string) ([]string, error)
}

// atomicSweepBatchSize bounds the candidates rechecked in one atomic sweep
// transaction. The transaction holds the store's single writer lock, so every
// foreground write waits for the whole batch. Batching amortizes the per-commit
// cost; a small batch keeps that wait short on a busy volume.
const atomicSweepBatchSize = 16

// Stats holds GC cycle statistics.
type Stats struct {
	// AtomicSweepCount and AtomicSweepDuration include atomic ownership rechecks.
	AtomicSweepCount    int
	AtomicSweepDuration time.Duration
	// NodesSwept is the number of nodes swept.
	NodesSwept int
	// UnreferencedNodeCount is the number of unreferenced node entries read.
	UnreferencedNodeCount int
	// RemoveNodeRefsCount is the number of nodes whose outgoing refs were removed.
	RemoveNodeRefsCount int
	// RemoveUnreferencedEdgeCount is the number of unreferenced marker edges removed.
	RemoveUnreferencedEdgeCount int
	// OnSweptCount is the number of onSwept callbacks run.
	OnSweptCount int
	// RemoveBlockCount is the number of physical block deletes attempted.
	RemoveBlockCount int
	// Duration is how long the GC cycle took.
	Duration time.Duration
	// UnreferencedScanDuration is time spent listing unreferenced nodes.
	UnreferencedScanDuration time.Duration
	// RemoveNodeRefsDuration is time spent removing outgoing node refs.
	RemoveNodeRefsDuration time.Duration
	// RemoveUnreferencedEdgeDuration is time spent removing unreferenced markers.
	RemoveUnreferencedEdgeDuration time.Duration
	// OnSweptDuration is time spent in onSwept callbacks.
	OnSweptDuration time.Duration
	// RemoveBlockDuration is time spent physically deleting block-backed nodes.
	RemoveBlockDuration time.Duration
}

// Collector sweeps unreferenced nodes from the ref graph.
//
// Nodes are marked unreferenced by GCStoreOps when they lose all
// incoming references. Collect iterates unreferenced nodes and deletes
// them. Deletion cascades: removing a node may orphan its children,
// which get marked unreferenced for the next iteration.
type Collector struct {
	refGraph RefGraphOps
	store    block.StoreOps
	onSwept  func(ctx context.Context, iri string) error
}

// NewCollector constructs a new GC collector.
// The store is the underlying physical store for block deletion.
// The onSwept callback is optional; if non-nil it is called for each
// node before physical deletion. Without atomic sweeping, the caller must
// serialize collection with reference publication for this graph and store.
func NewCollector(
	refGraph RefGraphOps,
	store block.StoreOps,
	onSwept func(context.Context, string) error,
) *Collector {
	return &Collector{
		refGraph: refGraph,
		store:    store,
		onSwept:  onSwept,
	}
}

// Collect sweeps all unreferenced nodes and removes their physical blocks.
func (c *Collector) Collect(ctx context.Context) (*Stats, error) {
	return c.collect(ctx, true)
}

// CollectGraphOnly sweeps unreferenced nodes from the graph without authorizing
// deletion from the physical store.
func (c *Collector) CollectGraphOnly(ctx context.Context) (*Stats, error) {
	return c.collect(ctx, false)
}

// collect loops until no unreferenced nodes remain, since removing a node may
// orphan its children.
func (c *Collector) collect(ctx context.Context, removeBlocks bool) (*Stats, error) {
	// Start timing and accumulate cycle statistics across cascaded sweeps.
	start := time.Now()
	stats := &Stats{}
	defer func() { stats.Duration = time.Since(start) }()

	// Scan unreferenced nodes until an iteration makes no progress.
	for {
		if err := ctx.Err(); err != nil {
			return stats, ctx.Err()
		}

		// Read the current unreferenced set and account for scan time.
		phaseStart := time.Now()
		nodes, err := c.refGraph.GetUnreferencedNodes(ctx)
		stats.UnreferencedScanDuration += time.Since(phaseStart)
		if err != nil {
			if ctx.Err() != nil {
				return stats, ctx.Err()
			}
			return stats, errors.Wrap(err, "get unreferenced nodes")
		}
		stats.UnreferencedNodeCount += len(nodes)
		if len(nodes) == 0 {
			break
		}

		// A stale snapshot must not delete a block rescued by an intervening
		// publication. Use the store's physical atomic sweep where available.
		// Graph-only and callback collectors keep their established contract.
		if atomic, ok := c.store.(AtomicSweepStore); ok && removeBlocks && c.onSwept == nil {
			swept, err := c.sweepAtomic(ctx, atomic, nodes, stats)
			if err != nil {
				return stats, err
			}
			if swept == 0 {
				break
			}
			continue
		}

		// Remove each eligible node's graph edges, callback, and physical block.
		var swept int
		for _, node := range nodes {
			if err := ctx.Err(); err != nil {
				return stats, ctx.Err()
			}

			if IsPermanentRoot(node) {
				continue
			}

			// Graph-only callers serialize mutations in their World transaction.
			// Recheck stale staging marks before removing any live dependencies.
			owned, err := c.refGraph.HasIncomingRefs(ctx, node)
			if err != nil {
				return stats, errors.Wrap(err, "recheck orphan ownership")
			}
			if owned {
				continue
			}

			// Remove all outgoing gc/ref edges and mark orphaned targets.
			phaseStart = time.Now()
			_, err = c.refGraph.RemoveNodeRefs(ctx, node, true)
			stats.RemoveNodeRefsDuration += time.Since(phaseStart)
			if err != nil {
				if ctx.Err() != nil {
					return stats, ctx.Err()
				}
				if !errors.Is(err, block.ErrNotFound) {
					return stats, errors.Wrap(err, "remove node refs")
				}
			}
			stats.RemoveNodeRefsCount++

			// Call onSwept callback.
			if c.onSwept != nil {
				phaseStart = time.Now()
				if err := c.onSwept(ctx, node); err != nil {
					stats.OnSweptDuration += time.Since(phaseStart)
					if ctx.Err() != nil {
						return stats, ctx.Err()
					}
					return stats, errors.Wrap(err, "on swept callback")
				}
				stats.OnSweptDuration += time.Since(phaseStart)
				stats.OnSweptCount++
			}

			// Physical deletion is valid only when this graph's reachability
			// scope owns the store.
			if ref, ok := ParseBlockIRI(node); ok && removeBlocks {
				phaseStart = time.Now()
				if err := c.store.RmBlock(ctx, ref); err != nil {
					stats.RemoveBlockDuration += time.Since(phaseStart)
					if ctx.Err() != nil {
						return stats, ctx.Err()
					}
					return stats, errors.Wrap(err, "remove block")
				}
				stats.RemoveBlockDuration += time.Since(phaseStart)
				stats.RemoveBlockCount++
			}

			// Remove the unreferenced -> node edge.
			phaseStart = time.Now()
			err = c.refGraph.RemoveRef(ctx, NodeUnreferenced, node)
			stats.RemoveUnreferencedEdgeDuration += time.Since(phaseStart)
			if err != nil {
				if ctx.Err() != nil {
					return stats, ctx.Err()
				}
				if !errors.Is(err, block.ErrNotFound) {
					return stats, errors.Wrap(err, "remove unreferenced edge")
				}
			}
			stats.RemoveUnreferencedEdgeCount++

			swept++
			stats.NodesSwept++
		}

		// Stop when only permanent roots or otherwise unsweepable nodes remain.
		// If no nodes were swept this iteration (e.g., all were
		// permanent roots), stop to avoid infinite loop.
		if swept == 0 {
			break
		}
	}

	return stats, nil
}

// sweepAtomic sweeps one candidate snapshot through the store's atomic sweep in
// bounded batches and returns the number of nodes removed.
func (c *Collector) sweepAtomic(ctx context.Context, atomic AtomicSweepStore, nodes []string, stats *Stats) (int, error) {
	candidates := slices.DeleteFunc(slices.Clone(nodes), IsPermanentRoot)
	var swept int
	for len(candidates) != 0 {
		batch := candidates[:min(len(candidates), atomicSweepBatchSize)]
		candidates = candidates[len(batch):]
		start := time.Now()
		removed, err := atomic.SweepUnreferenced(ctx, c.refGraph, batch)
		stats.AtomicSweepDuration += time.Since(start)
		stats.AtomicSweepCount += len(batch)
		if err != nil {
			if ctx.Err() != nil {
				return swept, ctx.Err()
			}
			return swept, errors.Wrap(err, "atomic sweep")
		}
		for _, node := range removed {
			swept++
			stats.NodesSwept++
			stats.RemoveNodeRefsCount++
			stats.RemoveUnreferencedEdgeCount++
			if _, ok := ParseBlockIRI(node); ok {
				stats.RemoveBlockCount++
			}
		}
	}
	return swept, nil
}
