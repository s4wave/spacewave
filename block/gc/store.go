package block_gc

import (
	"context"
	"runtime/trace"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// pendingRef is a buffered ref graph operation.
type pendingRef struct {
	source, target string
}

// WALAppender appends batched ref graph edge operations to a write-ahead
// log. When set on GCStoreOps, FlushPending writes to the WAL instead
// of calling ApplyRefBatch on the RefGraph directly.
type WALAppender interface {
	Append(ctx context.Context, adds, removes []RefEdge) error
}

// DeferFlushable is an alias for block.DeferFlushable.
type DeferFlushable = block.DeferFlushable

// GCStoreOps wraps a StoreOps with GC ref graph tracking.
//
// PutBlock and RecordBlockRefs are called from Transaction.Write's
// concurrent worker goroutines. Since the RefGraph shares the block
// cursor's mutex, writing to the RefGraph inside those goroutines
// would deadlock. Instead, GCStoreOps buffers the operations and
// they are flushed via FlushPending after Transaction.Write returns.
//
// When parentIRI is set, new blocks are tracked under parentIRI
// instead of the "unreferenced" staging node. This allows
// bucket-level ownership of blocks.
type GCStoreOps struct {
	store      block.StoreOps
	refGraph   RefGraphOps
	wal        WALAppender
	parentIRI  string
	flushTask  string
	deferFlush atomic.Int32

	mu             sync.Mutex
	pendingUnref   []string     // block IRIs needing parent/unreferenced -> block edges
	pendingRefs    []pendingRef // source -> target block ref edges
	pendingUnunref []string     // block IRIs to remove from unreferenced
}

const (
	defaultFlushTask = "hydra/block-gc/store/flush-pending"
	worldFlushTask   = "hydra/block-gc/store/flush-pending/world"
	bucketFlushTask  = "hydra/block-gc/store/flush-pending/bucket"
)

// WorldFlushTask returns the runtime trace task name for world-local GC flushes.
func WorldFlushTask() string {
	return worldFlushTask
}

// BucketFlushTask returns the runtime trace task name for bucket-level GC flushes.
func BucketFlushTask() string {
	return bucketFlushTask
}

// NewGCStoreOps wraps a StoreOps with GC ref graph tracking.
// New blocks are added under the "unreferenced" staging node.
func NewGCStoreOps(store block.StoreOps, refGraph RefGraphOps) *GCStoreOps {
	return NewGCStoreOpsWithTraceTask(store, refGraph, defaultFlushTask)
}

// NewGCStoreOpsWithTraceTask wraps a StoreOps with GC ref graph tracking and a
// specific runtime trace task name for FlushPending.
func NewGCStoreOpsWithTraceTask(store block.StoreOps, refGraph RefGraphOps, flushTask string) *GCStoreOps {
	if flushTask == "" {
		flushTask = defaultFlushTask
	}
	return &GCStoreOps{
		store:     store,
		refGraph:  refGraph,
		flushTask: flushTask,
	}
}

// NewGCStoreOpsWithParent wraps a StoreOps with GC ref graph tracking
// using a specific parent IRI. New blocks are tracked under parentIRI
// instead of the "unreferenced" staging node.
func NewGCStoreOpsWithParent(store block.StoreOps, refGraph RefGraphOps, parentIRI string) *GCStoreOps {
	return NewGCStoreOpsWithParentAndTraceTask(store, refGraph, parentIRI, defaultFlushTask)
}

// NewGCStoreOpsWithParentAndTraceTask wraps a StoreOps with GC ref graph
// tracking and a specific runtime trace task name for FlushPending.
func NewGCStoreOpsWithParentAndTraceTask(store block.StoreOps, refGraph RefGraphOps, parentIRI, flushTask string) *GCStoreOps {
	if flushTask == "" {
		flushTask = defaultFlushTask
	}
	return &GCStoreOps{
		store:     store,
		refGraph:  refGraph,
		parentIRI: parentIRI,
		flushTask: flushTask,
	}
}

// SetWALAppender sets the WAL appender for deferred ref graph updates.
// When set, FlushPending writes to the WAL instead of calling
// ApplyRefBatch on the RefGraph directly.
func (g *GCStoreOps) SetWALAppender(wal WALAppender) {
	g.wal = wal
}

// GetHashType returns the preferred hash type for the store.
func (g *GCStoreOps) GetHashType() hash.HashType {
	return g.store.GetHashType()
}

// GetRefGraph returns the underlying ref graph.
func (g *GCStoreOps) GetRefGraph() RefGraphOps {
	return g.refGraph
}

// GetStore returns the underlying store.
func (g *GCStoreOps) GetStore() block.StoreOps {
	return g.store
}

// PutBlock puts a block into the store and buffers a gc/ref edge for
// later flush if the block is new. When parentIRI is set, the edge
// is parentIRI -> block; otherwise unreferenced -> block.
func (g *GCStoreOps) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/store/put-block")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/store/put-block/store-put-block")
	ref, existed, err := g.store.PutBlock(taskCtx, data, opts)
	subtask.End()
	if err != nil {
		return nil, false, err
	}
	if !existed && ref != nil && !ref.GetEmpty() {
		_, subtask = trace.NewTask(ctx, "hydra/block-gc/store/put-block/buffer-pending-unref")
		iri := BlockIRI(ref)
		g.mu.Lock()
		g.pendingUnref = append(g.pendingUnref, iri)
		g.mu.Unlock()
		subtask.End()
	}
	return ref, existed, nil
}

// PutBlockBatch writes a batch of blocks through the inner store and buffers
// GC ref edges for all new non-tombstone blocks. When the inner store
// implements BatchPutStore, the batch flows through as a single lower-layer
// operation. Otherwise falls back to per-entry PutBlock/RmBlock.
func (g *GCStoreOps) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/store/put-block-batch")
	defer task.End()

	if batcher, ok := g.store.(block.BatchPutStore); ok {
		if err := batcher.PutBlockBatch(ctx, entries); err != nil {
			return err
		}
	} else {
		for _, entry := range entries {
			if entry.Tombstone {
				if err := g.store.RmBlock(ctx, entry.Ref.Clone()); err != nil {
					return err
				}
				continue
			}
			if _, _, err := g.store.PutBlock(ctx, entry.Data, &block.PutOpts{
				ForceBlockRef: entry.Ref.Clone(),
			}); err != nil {
				return err
			}
		}
	}

	// Buffer GC ref edges for non-tombstone blocks.
	g.mu.Lock()
	for _, entry := range entries {
		if entry.Tombstone || entry.Ref == nil || entry.Ref.GetEmpty() {
			continue
		}
		g.pendingUnref = append(g.pendingUnref, BlockIRI(entry.Ref))
	}
	g.mu.Unlock()

	return nil
}

// GetBlock gets a block with the given reference.
func (g *GCStoreOps) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return g.store.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists.
func (g *GCStoreOps) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return g.store.GetBlockExists(ctx, ref)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (g *GCStoreOps) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return g.store.StatBlock(ctx, ref)
}

// RmBlock cleans up the ref graph for a block without performing a
// physical delete. The Collector handles physical deletion. This
// removes all outgoing gc/ref edges from the block, removes the
// parent/unreferenced -> block edge, and cascades orphan detection
// to any targets that lost their last incoming reference.
//
// When parentIRI is set, the parentIRI -> block edge is buffered as
// a pending unref removal. When parentIRI is empty, the unreferenced
// -> block edge is removed directly.
func (g *GCStoreOps) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	iri := BlockIRI(ref)

	if _, err := g.refGraph.RemoveNodeRefs(ctx, iri, true); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "remove outgoing refs")
	}

	parent := g.parentIRI
	if parent == "" {
		parent = NodeUnreferenced
	}
	return g.refGraph.RemoveRef(ctx, parent, iri)
}

// RecordBlockRefs buffers block-to-block reference edges for later flush.
func (g *GCStoreOps) RecordBlockRefs(_ context.Context, source *block.BlockRef, targets []*block.BlockRef) error {
	sourceIRI := BlockIRI(source)
	g.mu.Lock()
	for _, t := range targets {
		if t == nil || t.GetEmpty() {
			continue
		}
		targetIRI := BlockIRI(t)
		g.pendingRefs = append(g.pendingRefs, pendingRef{sourceIRI, targetIRI})
		g.pendingUnunref = append(g.pendingUnunref, targetIRI)
	}
	g.mu.Unlock()
	return nil
}

// BeginDeferFlush enters a deferred-flush scope. While deferred,
// FlushPending returns immediately without flushing; pending
// operations accumulate in the buffer. Supports nesting.
// Also forwards to the inner store if it implements DeferFlushable,
// so nested GC layers (e.g. bucket-level gcOps inside bucketHandle)
// are also deferred.
func (g *GCStoreOps) BeginDeferFlush() {
	g.deferFlush.Add(1)
	if df, ok := g.store.(block.DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush exits a deferred-flush scope. When the outermost
// scope ends, calls FlushPending to flush all accumulated operations
// in one batch. Also forwards to the inner store.
func (g *GCStoreOps) EndDeferFlush(ctx context.Context) error {
	var innerErr error
	if df, ok := g.store.(block.DeferFlushable); ok {
		innerErr = df.EndDeferFlush(ctx)
	}
	if g.deferFlush.Add(-1) == 0 {
		if err := g.FlushPending(ctx); err != nil {
			return err
		}
	}
	return innerErr
}

// FlushPending writes all buffered PutBlock and RecordBlockRefs
// operations to the RefGraph, using batched ref graph updates when
// the implementation supports them. Must be called after
// Transaction.Write completes and the cursor mutex is no longer held.
//
// When a deferred-flush scope is active (via BeginDeferFlush),
// returns nil without flushing. The pending operations accumulate
// and are flushed when EndDeferFlush closes the outermost scope.
func (g *GCStoreOps) FlushPending(ctx context.Context) error {
	if g.deferFlush.Load() > 0 {
		return nil
	}

	taskName := g.flushTask
	if taskName == "" {
		taskName = defaultFlushTask
	}
	ctx, task := trace.NewTask(ctx, taskName)
	defer task.End()

	g.mu.Lock()
	unrefs := g.pendingUnref
	refs := g.pendingRefs
	ununrefs := g.pendingUnunref
	g.pendingUnref = nil
	g.pendingRefs = nil
	g.pendingUnunref = nil
	g.mu.Unlock()

	if len(unrefs) == 0 && len(refs) == 0 && len(ununrefs) == 0 {
		return nil
	}

	parent := g.parentIRI
	if parent == "" {
		parent = NodeUnreferenced
	}

	adds := make([]RefEdge, 0, len(unrefs)+len(refs))
	for _, iri := range unrefs {
		adds = append(adds, RefEdge{Subject: parent, Object: iri})
	}
	for _, r := range refs {
		adds = append(adds, RefEdge{Subject: r.source, Object: r.target})
	}

	removes := make([]RefEdge, 0, len(ununrefs))
	for _, iri := range ununrefs {
		removes = append(removes, RefEdge{Subject: NodeUnreferenced, Object: iri})
	}

	if g.wal != nil {
		if err := g.wal.Append(ctx, adds, removes); err != nil {
			if ctx.Err() != nil {
				return context.Canceled
			}
			return errors.Wrap(err, "flush WAL append")
		}
		return nil
	}

	if err := g.refGraph.ApplyRefBatch(ctx, adds, removes); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "flush ref batch")
	}
	return nil
}

// AddGCRef adds a gc/ref edge from subject to object and removes
// the unreferenced edge from the object (it now has a real reference).
func (g *GCStoreOps) AddGCRef(ctx context.Context, subject, object string) error {
	if err := g.refGraph.AddRef(ctx, subject, object); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "add gc ref")
	}
	return g.refGraph.RemoveRef(ctx, NodeUnreferenced, object)
}

// RemoveGCRef removes a gc/ref edge from subject to object and marks
// the object as orphaned if it has no remaining incoming references.
func (g *GCStoreOps) RemoveGCRef(ctx context.Context, subject, object string) error {
	if err := g.refGraph.RemoveRef(ctx, subject, object); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "remove gc ref")
	}
	if IsPermanentRoot(object) {
		return nil
	}
	has, err := g.refGraph.HasIncomingRefs(ctx, object)
	if err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "check incoming refs")
	}
	if !has {
		return g.refGraph.AddRef(ctx, NodeUnreferenced, object)
	}
	return nil
}

// _ is a type assertion
var (
	_ block.StoreOps         = ((*GCStoreOps)(nil))
	_ block.BlockRefRecorder = ((*GCStoreOps)(nil))
	_ block.BatchPutStore    = ((*GCStoreOps)(nil))
)
