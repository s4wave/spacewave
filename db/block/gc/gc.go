// Package block_gc implements garbage collection for Hydra block stores.
//
// The ref graph tracks reference edges using a single gc/ref predicate stored
// in a KVtx store. A node that is not a block and loses all incoming gc/ref
// edges is marked unreferenced, and the Collector sweeps it, removing its
// outgoing edges, which may cascade further orphans. Blocks keep one child
// list each instead of a reference count: a block pass marks from the roots
// and every node that is not a block, and sweeps the tracked blocks it did not
// reach, calling the onSwept callback and physically deleting them via the
// underlying store.
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

// AtomicSweepStore removes still-orphaned nodes and their physical blocks,
// serialized with publication.
type AtomicSweepStore interface {
	// SweepUnreferenced rechecks the current ownership of each candidate and
	// removes the nodes still orphaned. A candidate snapshot alone never
	// authorizes deletion. The graph argument binds the sweep to the
	// collector's reachability scope. The store sizes its transactions to
	// bound how long each holds its writer; a transaction removes its nodes,
	// their edges and their blocks together or not at all. It returns the
	// nodes removed by committed transactions, also when it returns an error.
	SweepUnreferenced(ctx context.Context, graph RefGraphOps, nodes []string) ([]string, error)
	// SweepUnreachedBlocks runs one block pass: it opens the pass, marks the
	// graph's blocks, and removes the tracked blocks the mark did not reach
	// in transactions that each first rescue the blocks writes adopted since
	// the pass opened. It sweeps nothing when the mark reached a block of
	// unknown references. It removes every unreached block before it
	// returns, because the mark walks the whole graph and a later pass would
	// pay for that walk again, and records the pass in stats.
	SweepUnreachedBlocks(ctx context.Context, graph RefGraphOps, stats *Stats) error
}

// YoungSweepStore runs minor passes, which sweep only the blocks written since
// the last pass, as RefGraph.MarkYoung describes.
type YoungSweepStore interface {
	// SweepYoungBlocks runs one minor pass: it opens a block pass, marks the
	// young blocks, and removes the tracked young blocks the mark did not
	// reach, as SweepUnreachedBlocks removes the unreached blocks. It does
	// nothing until a block pass in the same process swept everything it
	// marked, and records the pass in stats.
	SweepYoungBlocks(ctx context.Context, graph RefGraphOps, stats *Stats) error
}

// BlockMarker marks the blocks of a reference graph in one snapshot, as
// RefGraph.MarkBlocks describes.
type BlockMarker interface {
	// MarkBlocks marks the graph's blocks and lists the tracked blocks the
	// mark did not reach.
	MarkBlocks(ctx context.Context) (*BlockMark, error)
}

// BlockTracker reports which blocks a reference graph tracks, as
// RefGraph.TrackedBlocks describes.
type BlockTracker interface {
	// TrackedBlocks reports, for each ref, whether the graph tracks the block.
	TrackedBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
}

// Collector sweeps unreferenced nodes and unreachable blocks from the ref
// graph.
//
// Nodes that are not blocks are marked unreferenced when they lose all
// incoming references. A collection first sweeps them, cascading as removing
// a node orphans its targets, then runs a block pass over a graph that
// supports one: it marks the blocks and sweeps the tracked blocks the mark did
// not reach.
type Collector struct {
	refGraph RefGraphOps
	store    block.StoreOps
	onSwept  func(ctx context.Context, iri string) error

	// markInterval spaces the block passes of budgeted collections.
	markInterval time.Duration
	// markBackoff doubles markInterval once for each consecutive block pass
	// that found little to sweep, up to maxMarkBackoff.
	markBackoff uint
	// nextMark is when the next budgeted block pass is due.
	nextMark time.Time
}

// maxMarkBackoff caps the doublings of the mark interval at 8x.
const maxMarkBackoff = 3

// sparseMarkDivisor sets the yield below which a block pass backs off: one
// that finds fewer than 1/sparseMarkDivisor of its live blocks unreached.
const sparseMarkDivisor = 16

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

// SetMarkInterval spaces the block passes of CollectFor at least interval
// apart, counting from the end of each pass, since a mark reads the whole
// graph. The first pass is due at next, the NextMark a caller saved before a
// restart, so restarts neither postpone a pass nor repeat one; a zero next
// counts one interval from now, and a next past the longest gap is pulled in
// to it. Each pass that finds fewer than 1/16 of its live blocks unreached
// doubles the gap before the next, up to 8x; a pass that finds more resets
// it. Collect and CollectGraphOnly always run one.
func (c *Collector) SetMarkInterval(interval time.Duration, next time.Time) {
	now := time.Now()
	if next.IsZero() {
		next = now.Add(interval)
	}
	if latest := now.Add(interval << maxMarkBackoff); next.After(latest) {
		next = latest
	}
	c.markInterval, c.markBackoff, c.nextMark = interval, 0, next
}

// NextMark returns when the next budgeted block pass is due.
func (c *Collector) NextMark() time.Time {
	return c.nextMark
}

// scheduleMark backs the mark interval off after a pass that freed little: a
// sparse one, or one that blocks of unknown references stopped, which sweeps
// nothing and would mark the whole store again for nothing until their lists
// arrive. It resets the interval after a productive pass, keeps it after a
// failed one, and counts the next gap from now.
func (c *Collector) scheduleMark(stats *Stats, err error) {
	switch {
	case err != nil:
	case stats.UnknownBlocks != 0 || stats.UnreachedBlocks*sparseMarkDivisor < stats.LiveNodes:
		c.markBackoff = min(c.markBackoff+1, maxMarkBackoff)
	default:
		c.markBackoff = 0
	}
	c.nextMark = time.Now().Add(c.markInterval << c.markBackoff)
}

// collectChunk bounds the candidates a budgeted collection sweeps between
// checks of its budget.
const collectChunk = 4096

// Collect sweeps all unreferenced nodes and removes their physical blocks.
func (c *Collector) Collect(ctx context.Context) (*Stats, error) {
	return c.collect(ctx, true, 0)
}

// CollectFor sweeps unreferenced nodes until none remain or budget has
// elapsed, and sets Stats.More when it stopped for the budget. It sweeps at
// least one chunk of candidates, so repeated calls progress however long
// listing the candidates takes. When the queue is empty and the mark interval
// has passed since the last pass ended, it then runs a block pass, which the
// budget does not stop: the pass sweeps every block its mark found unreached.
// Between block passes it runs a minor pass where the store supports one.
func (c *Collector) CollectFor(ctx context.Context, budget time.Duration) (*Stats, error) {
	return c.collect(ctx, true, budget)
}

// CollectGraphOnly sweeps unreferenced nodes from the graph without authorizing
// deletion from the physical store.
func (c *Collector) CollectGraphOnly(ctx context.Context) (*Stats, error) {
	return c.collect(ctx, false, 0)
}

// collect sweeps the unreferenced nodes, then runs a block pass when one is
// due, or else a minor pass, stopping when a nonzero budget has elapsed.
func (c *Collector) collect(ctx context.Context, removeBlocks bool, budget time.Duration) (*Stats, error) {
	// Start timing and accumulate cycle statistics across cascaded sweeps.
	start := time.Now()
	stats := &Stats{}
	defer func() { stats.Duration = time.Since(start) }()
	spent := func() bool { return budget != 0 && time.Since(start) >= budget }

	// Sweep the queue, then the blocks.
	if err := c.collectQueue(ctx, removeBlocks, spent, stats); err != nil || stats.More {
		return stats, err
	}
	if budget == 0 || c.markInterval == 0 {
		return stats, c.collectBlocks(ctx, removeBlocks, stats)
	}
	if time.Now().Before(c.nextMark) {
		return stats, c.collectYoung(ctx, removeBlocks, stats)
	}
	err := c.collectBlocks(ctx, removeBlocks, stats)
	c.scheduleMark(stats, err)
	return stats, err
}

// collectYoung runs a minor pass where the store supports one.
func (c *Collector) collectYoung(ctx context.Context, removeBlocks bool, stats *Stats) error {
	// Sweep only through a store that sweeps atomically, as collectBlocks.
	young, ok := c.store.(YoungSweepStore)
	if !ok || !removeBlocks || c.onSwept != nil {
		return nil
	}
	err := young.SweepYoungBlocks(ctx, c.refGraph, stats)
	if err != nil && ctx.Err() == nil {
		err = errors.Wrap(err, "sweep young blocks")
	}
	return err
}

// collectQueue loops until no unreferenced nodes remain, since removing a node
// may orphan its targets, or until spent reports true.
func (c *Collector) collectQueue(ctx context.Context, removeBlocks bool, spent func() bool, stats *Stats) error {
	// Scan unreferenced nodes until an iteration makes no progress.
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Read the current unreferenced set and account for scan time.
		phaseStart := time.Now()
		nodes, err := c.refGraph.GetUnreferencedNodes(ctx)
		stats.UnreferencedScanDuration += time.Since(phaseStart)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.Wrap(err, "get unreferenced nodes")
		}
		stats.UnreferencedNodeCount += len(nodes)
		stats.Candidates = len(nodes)
		if len(nodes) == 0 {
			break
		}

		// A stale snapshot must not delete a block rescued by an intervening
		// publication. Use the store's physical atomic sweep where available.
		// Graph-only and callback collectors keep their established contract.
		if atomic, ok := c.store.(AtomicSweepStore); ok && removeBlocks && c.onSwept == nil {
			// Sweep the candidates in chunks, stopping at the budget.
			candidates := slices.DeleteFunc(slices.Clone(nodes), IsPermanentRoot)
			var swept int
			for len(candidates) != 0 {
				chunk := candidates[:min(len(candidates), collectChunk)]
				candidates = candidates[len(chunk):]
				n, err := c.sweepAtomic(ctx, atomic, chunk, stats)
				swept += n
				if err != nil {
					return err
				}
				if len(candidates) != 0 && spent() {
					stats.More = true
					return nil
				}
			}

			// Stop when the pass removed nothing, or report the cascade
			// left for the next call when the budget is spent.
			if swept == 0 {
				break
			}
			if spent() {
				stats.More = true
				return nil
			}
			continue
		}

		// Remove each eligible node's graph edges, callback, and physical block.
		var swept int
		for _, node := range nodes {
			if err := ctx.Err(); err != nil {
				return ctx.Err()
			}

			if IsPermanentRoot(node) {
				continue
			}

			// Graph-only callers serialize mutations in their World transaction.
			// Recheck stale staging marks before removing any live dependencies.
			owned, err := c.refGraph.HasIncomingRefs(ctx, node)
			if err != nil {
				return errors.Wrap(err, "recheck orphan ownership")
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
					return ctx.Err()
				}
				if !errors.Is(err, block.ErrNotFound) {
					return errors.Wrap(err, "remove node refs")
				}
			}
			stats.RemoveNodeRefsCount++

			// Call onSwept callback.
			if c.onSwept != nil {
				phaseStart = time.Now()
				if err := c.onSwept(ctx, node); err != nil {
					stats.OnSweptDuration += time.Since(phaseStart)
					if ctx.Err() != nil {
						return ctx.Err()
					}
					return errors.Wrap(err, "on swept callback")
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
						return ctx.Err()
					}
					return errors.Wrap(err, "remove block")
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
					return ctx.Err()
				}
				if !errors.Is(err, block.ErrNotFound) {
					return errors.Wrap(err, "remove unreferenced edge")
				}
			}
			stats.RemoveUnreferencedEdgeCount++

			swept++
			stats.NodesSwept++
			if spent() {
				stats.More = true
				return nil
			}
		}

		// Stop when only permanent roots or otherwise unsweepable nodes remain.
		// If no nodes were swept this iteration (e.g., all were
		// permanent roots), stop to avoid infinite loop.
		if swept == 0 {
			break
		}
	}
	return nil
}

// collectBlocks runs a block pass over a graph that supports one: through the
// store's atomic sweep where available, or else by marking the graph and
// sweeping the tracked blocks the mark did not reach, which requires the
// caller to serialize collection with reference publication. It sweeps every
// block the mark did not reach.
func (c *Collector) collectBlocks(ctx context.Context, removeBlocks bool, stats *Stats) error {
	// Sweep atomically where the store supports it.
	if atomic, ok := c.store.(AtomicSweepStore); ok && removeBlocks && c.onSwept == nil {
		err := atomic.SweepUnreachedBlocks(ctx, c.refGraph, stats)
		if err != nil && ctx.Err() == nil {
			err = errors.Wrap(err, "sweep unreached blocks")
		}
		return err
	}

	// Mark the blocks of a graph that supports it.
	marker, ok := c.refGraph.(BlockMarker)
	if !ok {
		return nil
	}
	start := time.Now()
	mark, err := marker.MarkBlocks(ctx)
	stats.MarkDuration += time.Since(start)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrap(err, "mark blocks")
	}
	stats.RecordMark(mark)
	if mark.Unknown != 0 {
		return nil
	}

	// Run each unreached block's callback, remove the physical block, then
	// its records last, so a failure leaves its child list for the next pass
	// to retry.
	for _, node := range mark.Unreached {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.onSwept != nil {
			if err := c.onSwept(ctx, node); err != nil {
				return errors.Wrap(err, "on swept callback")
			}
			stats.OnSweptCount++
		}
		if ref, ok := ParseBlockIRI(node); ok && removeBlocks {
			if err := c.store.RmBlock(ctx, ref); err != nil {
				return errors.Wrap(err, "remove block")
			}
			stats.RemoveBlockCount++
		}
		if _, err := c.refGraph.RemoveNodeRefs(ctx, node, true); err != nil {
			return errors.Wrap(err, "remove block refs")
		}
		stats.RemoveNodeRefsCount++
		stats.NodesSwept++
		stats.BlocksSwept++
	}
	return nil
}

// sweepAtomic sweeps candidates through the store's atomic sweep and returns
// the number of nodes removed.
func (c *Collector) sweepAtomic(ctx context.Context, atomic AtomicSweepStore, candidates []string, stats *Stats) (int, error) {
	// Sweep the candidates and account for the nodes removed, also on error.
	start := time.Now()
	removed, err := atomic.SweepUnreferenced(ctx, c.refGraph, candidates)
	stats.AtomicSweepDuration += time.Since(start)
	stats.AtomicSweepCount += len(candidates)
	for _, node := range removed {
		stats.NodesSwept++
		stats.RemoveNodeRefsCount++
		stats.RemoveUnreferencedEdgeCount++
		if _, ok := ParseBlockIRI(node); ok {
			stats.RemoveBlockCount++
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			return len(removed), ctx.Err()
		}
		return len(removed), errors.Wrap(err, "atomic sweep")
	}
	return len(removed), nil
}
