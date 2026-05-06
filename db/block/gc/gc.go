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
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// Stats holds GC cycle statistics.
type Stats struct {
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
// node before physical deletion.
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

// Collect sweeps all unreferenced nodes. Loops until no unreferenced
// nodes remain, since deleting a node may orphan its children.
func (c *Collector) Collect(ctx context.Context) (*Stats, error) {
	start := time.Now()
	stats := &Stats{}

	for {
		if err := ctx.Err(); err != nil {
			return stats, context.Canceled
		}

		phaseStart := time.Now()
		nodes, err := c.refGraph.GetUnreferencedNodes(ctx)
		stats.UnreferencedScanDuration += time.Since(phaseStart)
		if err != nil {
			if ctx.Err() != nil {
				return stats, context.Canceled
			}
			return stats, errors.Wrap(err, "get unreferenced nodes")
		}
		stats.UnreferencedNodeCount += len(nodes)
		if len(nodes) == 0 {
			break
		}

		var swept int
		for _, node := range nodes {
			if err := ctx.Err(); err != nil {
				return stats, context.Canceled
			}

			if IsPermanentRoot(node) {
				continue
			}

			// Remove all outgoing gc/ref edges and mark orphaned targets.
			phaseStart = time.Now()
			if _, err := c.refGraph.RemoveNodeRefs(ctx, node, true); err != nil {
				stats.RemoveNodeRefsDuration += time.Since(phaseStart)
				if ctx.Err() != nil {
					return stats, context.Canceled
				}
				return stats, errors.Wrap(err, "remove node refs")
			}
			stats.RemoveNodeRefsDuration += time.Since(phaseStart)
			stats.RemoveNodeRefsCount++

			// Remove the unreferenced -> node edge.
			phaseStart = time.Now()
			if err := c.refGraph.RemoveRef(ctx, NodeUnreferenced, node); err != nil {
				stats.RemoveUnreferencedEdgeDuration += time.Since(phaseStart)
				if ctx.Err() != nil {
					return stats, context.Canceled
				}
				return stats, errors.Wrap(err, "remove unreferenced edge")
			}
			stats.RemoveUnreferencedEdgeDuration += time.Since(phaseStart)
			stats.RemoveUnreferencedEdgeCount++

			// Call onSwept callback.
			if c.onSwept != nil {
				phaseStart = time.Now()
				if err := c.onSwept(ctx, node); err != nil {
					stats.OnSweptDuration += time.Since(phaseStart)
					if ctx.Err() != nil {
						return stats, context.Canceled
					}
					return stats, errors.Wrap(err, "on swept callback")
				}
				stats.OnSweptDuration += time.Since(phaseStart)
				stats.OnSweptCount++
			}

			// Physical delete for block-backed nodes.
			if ref, ok := ParseBlockIRI(node); ok {
				phaseStart = time.Now()
				if err := c.store.RmBlock(ctx, ref); err != nil {
					stats.RemoveBlockDuration += time.Since(phaseStart)
					if ctx.Err() != nil {
						return stats, context.Canceled
					}
					return stats, errors.Wrap(err, "remove block")
				}
				stats.RemoveBlockDuration += time.Since(phaseStart)
				stats.RemoveBlockCount++
			}

			swept++
			stats.NodesSwept++
		}

		// If no nodes were swept this iteration (e.g., all were
		// permanent roots), stop to avoid infinite loop.
		if swept == 0 {
			break
		}
	}

	stats.Duration = time.Since(start)
	return stats, nil
}
