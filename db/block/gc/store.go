package block_gc

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/hash"
)

// pendingRef is a buffered ref graph operation.
type pendingRef struct {
	source, target string
}

// WALAppender appends batched ref graph edge operations to a write-ahead
// log. When set on GCStoreOps, FlushPending writes to the WAL instead
// of calling ApplyRefBatch on the RefGraph directly.
type WALAppender interface {
	// Append durably journals one batch of edge additions and removals.
	Append(ctx context.Context, adds, removes []RefEdge) error
	// GetPendingOutgoingRefs returns the targets of journaled edges from node
	// that the ref graph has not applied yet, net of journaled removals.
	GetPendingOutgoingRefs(ctx context.Context, node string) ([]string, error)
}

// GCStoreOps wraps a StoreOps with GC ref graph tracking.
//
// PutBlock is called from Transaction.Write's concurrent worker goroutines.
// Since the RefGraph shares the block cursor's mutex, writing to the RefGraph
// inside those goroutines would deadlock. Instead, GCStoreOps buffers the
// operations and they are flushed via FlushPending after Transaction.Write
// returns.
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
	deferFlush atomic.Int64

	// flushMu preserves delivery order and joins overlapping flushes.
	flushMu         csync.Mutex
	mu              sync.Mutex
	pendingReleases map[string]struct{} // latest parent releases, canceled by a later put
	pendingUnref    []string            // block IRIs needing parent/unreferenced -> block edges
	pendingRefs     []pendingRef        // source -> target block ref edges
	pendingUnunref  []string            // block IRIs to remove from unreferenced
	pendingAdds     []RefEdge           // normalized add edges left by a failed direct flush
	pendingRemoves  []RefEdge           // normalized remove edges left by a failed direct flush
}

type storeTrackingDisabledContextKey struct{}

const (
	defaultFlushTask = "hydra/block-gc/store/flush-pending"
	worldFlushTask   = "hydra/block-gc/store/flush-pending/world"
	bucketFlushTask  = "hydra/block-gc/store/flush-pending/bucket"
)

func disableStoreTracking(ctx context.Context) context.Context {
	return context.WithValue(ctx, storeTrackingDisabledContextKey{}, true)
}

func storeTrackingDisabled(ctx context.Context) bool {
	disabled, _ := ctx.Value(storeTrackingDisabledContextKey{}).(bool)
	return disabled
}

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
// ApplyRefBatch on the RefGraph directly. Configure it before concurrent use.
func (g *GCStoreOps) SetWALAppender(wal WALAppender) {
	g.wal = wal
}

// HasWALAppender returns whether this GC store writes flushes to a WAL.
func (g *GCStoreOps) HasWALAppender() bool {
	return g != nil && g.wal != nil
}

// GetHashType returns the preferred hash type for the store.
func (g *GCStoreOps) GetHashType() hash.HashType {
	return g.store.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (g *GCStoreOps) GetSupportedFeatures() block.StoreFeature {
	return g.store.GetSupportedFeatures()
}

// GetRefGraph returns the underlying ref graph.
func (g *GCStoreOps) GetRefGraph() RefGraphOps {
	return g.refGraph
}

// GetStore returns the underlying store.
func (g *GCStoreOps) GetStore() block.StoreOps {
	return g.store
}

// BeginReadOperation opens a read scope on the inner store.
func (g *GCStoreOps) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := g.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	scoped := &GCStoreOps{
		store:     store,
		refGraph:  g.refGraph,
		wal:       g.wal,
		parentIRI: g.parentIRI,
		flushTask: g.flushTask,
	}
	scoped.deferFlush.Store(g.deferFlush.Load())
	return scoped, release, nil
}

// PutBlock puts a block into the store and buffers a gc/ref edge for later
// flush. A parent owns every non-empty block it writes. Without a parent, only
// new blocks are staged under unreferenced.
func (g *GCStoreOps) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/store/put-block")
	defer task.End()

	putOpts, syncRequested := block.PutOptsWithoutSync(opts)
	finish := func(ref *block.BlockRef, existed bool) (*block.BlockRef, bool, error) {
		if syncRequested {
			if _, err := g.Sync(ctx); err != nil {
				return ref, existed, err
			}
		}
		return ref, existed, nil
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/store/put-block/store-put-block")
	ref, existed, err := g.store.PutBlock(taskCtx, data, putOpts)
	subtask.End()
	if err != nil {
		return nil, false, err
	}
	if storeTrackingDisabled(ctx) {
		return finish(ref, existed)
	}
	if ref != nil && !ref.GetEmpty() && (g.parentIRI != "" || !existed) {
		_, subtask = trace.NewTask(ctx, "hydra/block-gc/store/put-block/buffer-pending-unref")
		iri := BlockIRI(ref)
		g.mu.Lock()
		g.pendingUnref = append(g.pendingUnref, iri)
		delete(g.pendingReleases, iri)
		g.mu.Unlock()
		subtask.End()
	}
	var refs []*block.BlockRef
	if putOpts != nil {
		refs = putOpts.GetRefs()
	}
	if ref != nil && !ref.GetEmpty() && len(refs) != 0 {
		g.bufferBlockRefs(ref, refs)
	}
	return finish(ref, existed)
}

// PutBlockBatch writes a batch of blocks through the inner store and buffers
// GC ref edges for all non-tombstone blocks. The inner store decides whether
// the batch flows through a native path or an internal fallback.
//
// Tombstone entries release parent ownership through GCStoreOps.RmBlock.
// The collector owns outgoing edges and physical deletion. Existing entries
// skip unreferenced staging; a configured parent owns every block it writes.
func (g *GCStoreOps) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/store/put-block-batch")
	defer task.End()

	if storeTrackingDisabled(ctx) {
		return g.store.PutBlockBatch(ctx, entries)
	}

	var puts []*block.PutBatchEntry
	for _, entry := range entries {
		if !entry.Tombstone {
			puts = append(puts, entry)
		}
	}

	var existing []bool
	if g.parentIRI == "" && len(puts) != 0 {
		// Staging an existing block under unreferenced could revive a block
		// that already has real parents.
		checkCtx, checkTask := trace.NewTask(ctx, "hydra/block-gc/store/put-block-batch/check-existing")
		refs := make([]*block.BlockRef, len(puts))
		for i, entry := range puts {
			if entry.Ref == nil || entry.Ref.GetEmpty() {
				continue
			}
			refs[i] = entry.Ref
		}
		exists, err := g.store.GetBlockExistsBatch(checkCtx, refs)
		if err != nil {
			checkTask.End()
			return err
		}
		existing = exists
		checkTask.End()
	}

	writeCtx, writeTask := trace.NewTask(ctx, "hydra/block-gc/store/put-block-batch/inner-put-block-batch")
	if err := g.store.PutBlockBatch(writeCtx, puts); err != nil {
		writeTask.End()
		return err
	}
	writeTask.End()

	_, subtask := trace.NewTask(ctx, "hydra/block-gc/store/put-block-batch/buffer-pending-unref")
	g.mu.Lock()
	putIndex := 0
	for _, entry := range entries {
		if entry.Tombstone {
			if g.parentIRI != "" && entry.Ref != nil && !entry.Ref.GetEmpty() {
				g.bufferReleaseLocked(BlockIRI(entry.Ref))
			}
			continue
		}
		i := putIndex
		putIndex++
		if entry.Ref == nil || entry.Ref.GetEmpty() {
			continue
		}
		if g.parentIRI != "" || !existing[i] {
			iri := BlockIRI(entry.Ref)
			g.pendingUnref = append(g.pendingUnref, iri)
			delete(g.pendingReleases, iri)
		}
		if len(entry.Refs) != 0 {
			g.bufferBlockRefsLocked(entry.Ref, entry.Refs)
		}
	}
	g.mu.Unlock()
	subtask.End()

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

// GetBlockExistsBatch checks whether each block exists.
func (g *GCStoreOps) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return g.store.GetBlockExistsBatch(ctx, refs)
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (g *GCStoreOps) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return g.store.StatBlock(ctx, ref)
}

// RmBlock buffers release of this store's parent ownership. Outgoing edges
// belong to the immutable block and remain until the collector sweeps it.
// FlushPending delivers the release. An unparented store owns no reference
// to release; its staging mark remains.
func (g *GCStoreOps) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	iri := BlockIRI(ref)
	if iri == "" || g.parentIRI == "" {
		return nil
	}
	g.mu.Lock()
	g.bufferReleaseLocked(iri)
	g.mu.Unlock()
	return nil
}

func (g *GCStoreOps) bufferReleaseLocked(iri string) {
	if g.pendingReleases == nil {
		g.pendingReleases = make(map[string]struct{})
	}
	g.pendingReleases[iri] = struct{}{}
}

// Sync forwards the durability barrier to the inner store. Applying buffered
// ref-graph edges stays an explicit FlushPending call, not a Sync side effect,
// because FlushPending must run with the cursor mutex released.
func (g *GCStoreOps) Sync(ctx context.Context) (bool, error) {
	return g.store.Sync(ctx)
}

func (g *GCStoreOps) bufferBlockRefs(source *block.BlockRef, targets []*block.BlockRef) {
	g.mu.Lock()
	g.bufferBlockRefsLocked(source, targets)
	g.mu.Unlock()
}

func (g *GCStoreOps) bufferBlockRefsLocked(source *block.BlockRef, targets []*block.BlockRef) {
	sourceIRI := BlockIRI(source)
	for _, t := range targets {
		if t == nil || t.GetEmpty() {
			continue
		}
		targetIRI := BlockIRI(t)
		g.pendingRefs = append(g.pendingRefs, pendingRef{sourceIRI, targetIRI})
		g.pendingUnunref = append(g.pendingUnunref, targetIRI)
	}
}

// BeginDeferFlush enters a deferred-flush scope. While deferred,
// FlushPending returns immediately without flushing; pending
// operations accumulate in the buffer. Supports nesting.
// Also forwards to the inner store so nested GC layers (e.g. bucket-level
// gcOps inside bucketHandle) are also deferred.
func (g *GCStoreOps) BeginDeferFlush() {
	g.deferFlush.Add(1)
	block.BeginDeferFlush(g.store)
}

// EndDeferFlush exits a deferred-flush scope. When the outermost
// scope ends, calls FlushPending to flush all accumulated operations
// in one batch. Also forwards to the inner store.
func (g *GCStoreOps) EndDeferFlush(ctx context.Context) error {
	var depth int64
	for {
		depth = g.deferFlush.Load()
		if depth == 0 {
			return errors.New("block gc: EndDeferFlush called more than BeginDeferFlush")
		}
		if g.deferFlush.CompareAndSwap(depth, depth-1) {
			depth--
			break
		}
	}
	innerErr := block.EndDeferFlush(ctx, g.store)
	if depth == 0 {
		if err := g.FlushPending(ctx); err != nil {
			return err
		}
	}
	return innerErr
}

// FlushPending writes all buffered PutBlock operations to the RefGraph, using
// batched ref graph updates when
// the implementation supports them. Must be called after
// Transaction.Write completes and the cursor mutex is no longer held.
//
// When a deferred-flush scope is active (via BeginDeferFlush),
// returns nil without flushing. The pending operations accumulate
// and are flushed when EndDeferFlush closes the outermost scope.
func (g *GCStoreOps) FlushPending(ctx context.Context) error {
	release, err := g.flushMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if g.deferFlush.Load() > 0 {
		return nil
	}

	taskName := g.flushTask
	if taskName == "" {
		taskName = defaultFlushTask
	}
	ctx, task := trace.NewTask(ctx, taskName)
	defer task.End()

	// Finish an older failed delivery before taking newer ownership changes.
	g.mu.Lock()
	retryAdds, retryRemoves := g.pendingAdds, g.pendingRemoves
	g.pendingAdds, g.pendingRemoves = nil, nil
	g.mu.Unlock()
	if len(retryAdds)+len(retryRemoves) != 0 {
		if err := g.flushRefEdges(ctx, retryAdds, retryRemoves); err != nil {
			return err
		}
	}

	g.mu.Lock()
	unrefs := g.pendingUnref
	refs := g.pendingRefs
	ununrefs := g.pendingUnunref
	releases := g.pendingReleases
	g.pendingUnref = nil
	g.pendingRefs = nil
	g.pendingUnunref = nil
	g.pendingReleases = nil
	g.mu.Unlock()

	if len(unrefs) == 0 && len(refs) == 0 && len(ununrefs) == 0 &&
		len(releases) == 0 {
		return nil
	}
	path := "direct"
	if g.wal != nil {
		path = "wal"
	}
	trace.Logf(
		ctx,
		"hydra/block-gc/store/flush-pending/pending",
		"path=%s unrefs=%d refs=%d ununrefs=%d releases=%d",
		path,
		len(unrefs),
		len(refs),
		len(ununrefs),
		len(releases),
	)

	parent := g.parentIRI
	if parent == "" {
		parent = NodeUnreferenced
	}
	// Parent-backed writes remove their staging edge in the same batch as the
	// parent edge. RefGraph treats removal of a missing edge as a no-op.
	adds := make([]RefEdge, 0, len(unrefs)+len(refs)+len(releases))
	removes := make([]RefEdge, 0, len(ununrefs)+len(unrefs)+len(releases))
	for _, iri := range unrefs {
		adds = append(adds, RefEdge{Subject: parent, Object: iri})
		if _, released := releases[iri]; g.parentIRI != "" && !released {
			removes = append(removes, RefEdge{Subject: NodeUnreferenced, Object: iri})
		}
	}
	for _, r := range refs {
		adds = append(adds, RefEdge{Subject: r.source, Object: r.target})
	}
	for _, iri := range ununrefs {
		removes = append(removes, RefEdge{Subject: parent, Object: iri})
	}
	for iri := range releases {
		// Materialize and release the owner together, so a never-published
		// put also receives an orphan mark after the final owner check.
		edge := RefEdge{Subject: parent, Object: iri}
		adds = append(adds, edge)
		removes = append(removes, edge)
	}
	preNormalizeAdds := len(adds)
	preNormalizeRemoves := len(removes)
	_, normalizeTask := trace.NewTask(ctx, "hydra/block-gc/store/flush-pending/normalize")
	adds, removes = deduplicateRefEdges(adds), deduplicateRefEdges(removes)
	normalizeTask.End()
	trace.Logf(
		ctx,
		"hydra/block-gc/store/flush-pending/normalized",
		"path=%s adds_before=%d removes_before=%d adds_after=%d removes_after=%d",
		path,
		preNormalizeAdds,
		preNormalizeRemoves,
		len(adds),
		len(removes),
	)
	if len(adds) == 0 && len(removes) == 0 {
		return nil
	}

	return g.flushRefEdges(ctx, adds, removes)
}

// flushRefEdges retains every undelivered edge before reporting an error.
func (g *GCStoreOps) flushRefEdges(ctx context.Context, adds, removes []RefEdge) error {
	if g.wal != nil {
		walCtx, walTask := trace.NewTask(ctx, "hydra/block-gc/store/flush-pending/wal-append")
		if err := g.wal.Append(walCtx, adds, removes); err != nil {
			walTask.End()
			g.rebufferEdges(adds, removes)
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.Wrap(err, "flush WAL append")
		}
		walTask.End()
		return nil
	}

	batchCtx, batchTask := trace.NewTask(ctx, "hydra/block-gc/store/flush-pending/apply-ref-batch")
	if err := g.refGraph.ApplyRefBatch(batchCtx, adds, removes); err != nil {
		batchTask.End()
		if remainderAdds, remainderRemoves, ok := RefBatchRemainder(err); ok {
			g.rebufferEdges(remainderAdds, remainderRemoves)
		} else {
			g.rebufferEdges(adds, removes)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrap(err, "flush ref batch")
	}
	batchTask.End()
	return nil
}

func (g *GCStoreOps) rebufferEdges(adds, removes []RefEdge) {
	if len(adds) == 0 && len(removes) == 0 {
		return
	}
	g.mu.Lock()
	g.pendingAdds = append(slices.Clone(adds), g.pendingAdds...)
	g.pendingRemoves = append(slices.Clone(removes), g.pendingRemoves...)
	g.mu.Unlock()
}

// deduplicateRefEdges preserves operation order and opposing changes for the
// RefGraph to derive orphan marks before collapsing the final delta.
func deduplicateRefEdges(edges []RefEdge) []RefEdge {
	seen := make(map[RefEdge]struct{}, len(edges))
	out := make([]RefEdge, 0, len(edges))
	for _, edge := range edges {
		if _, ok := seen[edge]; !ok {
			seen[edge] = struct{}{}
			out = append(out, edge)
		}
	}
	return out
}

func normalizeRefEdges(adds, removes []RefEdge) ([]RefEdge, []RefEdge) {
	removeKeys := make(map[RefEdge]struct{}, len(removes))
	normalizedRemoves := make([]RefEdge, 0, len(removes))
	for _, edge := range removes {
		key := edge
		if _, ok := removeKeys[key]; ok {
			continue
		}
		removeKeys[key] = struct{}{}
		normalizedRemoves = append(normalizedRemoves, edge)
	}

	addKeys := make(map[RefEdge]struct{}, len(adds))
	normalizedAdds := make([]RefEdge, 0, len(adds))
	for _, edge := range adds {
		key := edge
		if _, removed := removeKeys[key]; removed {
			continue
		}
		if _, ok := addKeys[key]; ok {
			continue
		}
		addKeys[key] = struct{}{}
		normalizedAdds = append(normalizedAdds, edge)
	}
	return normalizedAdds, normalizedRemoves
}

// AddGCRef adds a gc/ref edge from subject to object and removes
// the unreferenced edge from the object (it now has a real reference).
func (g *GCStoreOps) AddGCRef(ctx context.Context, subject, object string) error {
	return g.refGraph.ApplyRefBatch(ctx,
		[]RefEdge{{Subject: subject, Object: object}},
		[]RefEdge{{Subject: NodeUnreferenced, Object: object}},
	)
}

// RemoveGCRef removes a gc/ref edge from subject to object and marks
// the object as orphaned if it has no remaining incoming references.
func (g *GCStoreOps) RemoveGCRef(ctx context.Context, subject, object string) error {
	if err := g.refGraph.ApplyRefBatch(ctx, nil, []RefEdge{{
		Subject: subject,
		Object:  object,
	}}); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.Wrap(err, "remove gc ref")
	}
	return nil
}

// _ is a type assertion
var (
	_ block.StoreOps     = (*GCStoreOps)(nil)
	_ block.DeferFlusher = (*GCStoreOps)(nil)
)
