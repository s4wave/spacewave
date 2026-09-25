package block

import (
	"context"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/aperturerobotics/util/conc"
	"github.com/pkg/errors"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/net/hash"
)

// maxWriteConcurrency is the maximum concurrency for PutBlock calls.
var maxWriteConcurrency = runtime.GOMAXPROCS(0)

// maxEncodeConcurrency is the maximum concurrency for hashing & marshaling blocks.
var maxEncodeConcurrency = maxWriteConcurrency

// transactionReachableNode tracks one node in the marshal reachability graph.
type transactionReachableNode struct {
	// from lists child nodes whose encoding must finish before this node.
	from []int64
	// encodeDone is closed when encoding this node is done.
	encodeDone chan struct{}
}

// Transaction tracks refs traversed between blocks, batching writes and
// propagating changes through the merkle graph.
//
// The decoded object form of the block can be stored / attached to a block
// handle. Changes are written to storage with a topological reference sort.
//
// Empty blocks are not written to storage: they are instead represented with a
// nil BlockRef. SetBlockRef should handle nil BlockRef objects correctly.
type Transaction struct {
	// store is the block store handle.
	store StoreOps
	// xfrm is an optional block transformer.
	xfrm Transformer
	// root is the root reference.
	root *handle
	// mtx guards the transaction's cursor graph and write operations.
	mtx sync.Mutex
	// blockGraph is the graph of blocks.
	blockGraph *BlockGraph
	// putOpts are put options with a resolved hash type.
	putOpts *PutOpts
	// dirty indicates whether anything changed in the transaction.
	dirty bool
	// bufferedStoreSettings overrides the default BufferedStore settings used
	// inside WriteAtRoot. nil uses the defaults.
	bufferedStoreSettings *BufferedStoreSettings
	// decodedBlocks is borrowed from the owning object lifecycle.
	decodedBlocks *DecodedBlockCache
	// stagedStore buffers content-addressed sub-tree blocks until a write drains
	// them before the root that references them.
	stagedStore atomic.Pointer[BufferedStore]
	// writeBuffer is borrowed by sub-transactions that must add blocks to a
	// transaction-level staging store without draining it themselves.
	writeBuffer *BufferedStore
}

// NewTransaction builds a new transaction with a root cursor. The transformer,
// root reference, and put options are optional.
func NewTransaction(
	store StoreOps,
	transformer Transformer,
	rootRef *BlockRef,
	putOpts *PutOpts,
) (*Transaction, *Cursor) {
	// Copy caller options so each write can assign its own block reference.
	opts := &PutOpts{}
	if putOpts != nil {
		opts = putOpts.CloneVT()
	}
	opts.ForceBlockRef = nil
	putOpts = opts

	// Resolve the hash type from explicit options, the store, or the default.
	hashType := putOpts.GetHashType()
	if hashType == 0 && store != nil {
		hashType = store.GetHashType()
	}
	if hashType == 0 {
		hashType = DefaultHashType
	}
	putOpts.HashType = hashType

	// Attach the root to its own cursor graph.
	t := &Transaction{
		store:      store,
		xfrm:       transformer,
		root:       &handle{ref: rootRef},
		blockGraph: NewBlockGraph(),
		putOpts:    putOpts,
	}
	t.root.Node = t.blockGraph.NewNode()
	t.blockGraph.AddNode(t.root)
	cs := newCursor(t, t.root, nil)
	return t, cs
}

// GetBlockGraph returns a handle to the internal block graph state.
// Do not modify this, used for analysis.
func (t *Transaction) GetBlockGraph() *BlockGraph {
	return t.blockGraph
}

// GetTransformer returns the transaction's block transformer.
func (t *Transaction) GetTransformer() Transformer {
	if t == nil {
		return nil
	}
	return t.xfrm
}

// GetPutOpts returns the transaction's put options.
func (t *Transaction) GetPutOpts() *PutOpts {
	if t == nil {
		return nil
	}
	return t.putOpts
}

// GetStoreOps returns the transaction's store operations.
func (t *Transaction) GetStoreOps() StoreOps {
	if t == nil {
		return nil
	}
	return t.store
}

// SetStoreOps replaces the transaction's store implementation.
// Used to swap in GCStoreOps after the RefGraph is available.
func (t *Transaction) SetStoreOps(store StoreOps) {
	if t == nil {
		return
	}
	t.store = store
}

// StageWrites returns the transaction staging store, creating it on first use.
// SetStoreOps must not be called after staging begins.
func (t *Transaction) StageWrites(ctx context.Context, inner StoreOps) *BufferedStore {
	if t == nil || inner == nil {
		return nil
	}
	if staged := t.stagedStore.Load(); staged != nil {
		return staged
	}
	staged := NewBufferedStoreWithSettings(ctx, inner, t.bufferedStoreSettings)
	if t.stagedStore.CompareAndSwap(nil, staged) {
		return staged
	}
	return t.stagedStore.Load()
}

// GetStagedStore returns the transaction staging store when one exists.
func (t *Transaction) GetStagedStore() *BufferedStore {
	if t == nil {
		return nil
	}
	return t.stagedStore.Load()
}

// DiscardStagedWrites drops blocks that have not been published.
func (t *Transaction) DiscardStagedWrites() {
	if t != nil {
		t.stagedStore.Store(nil)
	}
}

// SetWriteBuffer borrows a buffer that WriteAtRoot must not drain.
func (t *Transaction) SetWriteBuffer(buffer *BufferedStore) {
	if t != nil {
		t.writeBuffer = buffer
	}
}

// SetDecodedBlockCache sets the lifecycle-owned decoded-block cache borrowed by this transaction.
func (t *Transaction) SetDecodedBlockCache(cache *DecodedBlockCache) {
	if t == nil {
		return
	}

	// Replace the borrowed cache under the cursor-graph lock.
	t.mtx.Lock()
	t.decodedBlocks = cache
	t.mtx.Unlock()
}

// SetBufferedStoreSettings overrides the BufferedStore settings used to wrap
// the write store inside WriteAtRoot. Pass nil to reset to defaults. This must
// be called before Write/WriteAtRoot begins committing for the override to
// take effect on that commit.
func (t *Transaction) SetBufferedStoreSettings(s *BufferedStoreSettings) {
	// Ignore settings for an absent transaction.
	if t == nil {
		return
	}

	// Copy settings so later caller changes cannot alter this transaction.
	t.mtx.Lock()
	if s == nil {
		t.bufferedStoreSettings = nil
		t.mtx.Unlock()
		return
	}
	sCopy := *s
	t.bufferedStoreSettings = &sCopy
	t.mtx.Unlock()
}

// SetRoot sets the root of the transaction to a different position.
// Clears all parent blocks from the new root.
func (t *Transaction) SetRoot(cursor *Cursor) error {
	// Only cursors from this transaction can become its root.
	if t == nil {
		return nil
	}
	if cursor.t != nil && cursor.t != t {
		return errors.New("cursor block transaction mismatch")
	}

	// Detach the new root from its parents and mark it for writing.
	t.mtx.Lock()
	defer t.mtx.Unlock()
	_ = cursor.removeParent(nil)
	t.root = cursor.pos
	t.dirty = true
	cursor.pos.dirty = true
	return nil
}

// Write writes the dirty blocks to the store, propagating reference changes up
// the tree. Clears the blocks cache if clearTree is set, otherwise the updated
// references are written to the cursor tree. The final block in the event list
// will be the new root. The new root cursor is returned. Blocks that are not
// referenced by the root directly or indirectly are "cut" and removed.
//
// Note: after Write with clearTree, use the new returned rcursor only.
func (t *Transaction) Write(ctx context.Context, clearTree bool) (
	res *BlockRef,
	rcursor *Cursor,
	rerr error,
) {
	return t.WriteAtRoot(ctx, clearTree, nil)
}

// WriteAtRoot writes dirty blocks to the store starting from a sub-tree root.
// If subRoot is nil, writes from the transaction root (same as Write).
// If subRoot is non-nil, writes only the sub-tree rooted at that cursor.
// Blocks outside the sub-tree are not touched. After writing, the sub-tree
// nodes are non-dirty with refs set, and their block data is freed if
// clearTree is set. The parent transaction's Write() will skip these nodes.
func (t *Transaction) WriteAtRoot(ctx context.Context, clearTree bool, subRoot *Cursor) (
	res *BlockRef,
	rcursor *Cursor,
	rerr error,
) {
	// Trace the complete write, including draining and worker settlement.
	ctx, task := trace.NewTask(ctx, "hydra/block/transaction/write-at-root")
	defer task.End()

	if t == nil {
		return nil, nil, tx.ErrNotWrite
	}

	// Select the full transaction root or the requested subtree.
	writeRoot := t.root
	if subRoot != nil {
		if subRoot.t != nil && subRoot.t != t {
			return nil, nil, errors.New("cursor block transaction mismatch")
		}
		writeRoot = subRoot.pos
	}

	// Close deferred GC flushing after unlocking the cursor graph: RefGraph
	// may share that lock. Keep the caller context for this final flush.
	var deferFlushActive bool
	deferFlushCtx := ctx
	writeStore := t.store
	defer func() {
		if deferFlushActive {
			if err := EndDeferFlush(deferFlushCtx, writeStore); err != nil && rerr == nil {
				rerr = err
			}
		}
	}()

	// Serialize graph mutation and restore the returned cursor after writing.
	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer func() {
		// Subtree writes leave the surrounding transaction graph intact.
		if clearTree && subRoot == nil {
			t.clearData()
		}
		if rcursor == nil {
			rcursor = newCursor(t, writeRoot, nil)
		}
	}()

	// Clean transactions neither write nor flush another transaction's refs.
	if !t.dirty {
		return writeRoot.ref, nil, nil
	}

	// buffered is the per-write coalescer wrapping the write store. A borrowed
	// write buffer collects blocks for its parent transaction and is not drained
	// by this write.
	staged := t.stagedStore.Load()
	var buffered *BufferedStore
	drainBuffered := false
	existingBuffer, _ := writeStore.(*BufferedStore)
	switch {
	case t.writeBuffer != nil:
		buffered = t.writeBuffer
		writeStore = buffered
	case existingBuffer != nil && t.bufferedStoreSettings == nil:
		// The caller already owns writeback and its final durability fence.
		// Draining another coalescer into that buffer only repeats hashing,
		// cloning, and queue bookkeeping for every block.
		buffered = existingBuffer
	case staged != nil && staged.inner == writeStore:
		// The staging store already coalesces into the write store with the
		// same settings. Queueing this write's blocks behind the staged
		// sub-tree blocks drains both in one pass, in arrival order, so no
		// root reaches the store before the blocks it references.
		buffered = staged
		writeStore = buffered
		drainBuffered = true
	case writeStore != nil:
		buffered = NewBufferedStoreWithSettings(ctx, writeStore, t.bufferedStoreSettings)
		writeStore = buffered
		drainBuffered = true
	}

	// Publish other staged sub-tree blocks before encoding roots that
	// reference them. The staging store can intentionally bypass the GC WAL
	// wrapper used by page writes, so it drains separately from the coalescer.
	if staged != nil && staged != buffered {
		_, subtask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/drain-staged-store")
		staged.logPendingShape(ctx, "hydra/block/transaction/write-at-root/drain-staged-store/before")
		err := staged.drainAll(ctx)
		staged.logPendingShape(ctx, "hydra/block/transaction/write-at-root/drain-staged-store/after")
		subtask.End()
		if err != nil {
			return nil, nil, err
		}
		t.stagedStore.CompareAndSwap(staged, nil)
	}

	// Batch GC reference updates for the dirty write.
	if writeStore != nil {
		deferFlushActive = true
		BeginDeferFlush(writeStore)
	}

	// Cancel and join work before releasing transaction state.
	ctx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// Mark blocks reachable from the write root; full writes cut other blocks.
	reachable := make(map[int64]transactionReachableNode, 1)
	_, subtask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/mark-reachable")
	{
		var reachableEdges int
		nodStack := []GraphNode{writeRoot}
		for len(nodStack) != 0 {
			nn := nodStack[len(nodStack)-1]
			nodStack = nodStack[:len(nodStack)-1]
			nnID := nn.ID()
			if _, ok := reachable[nnID]; ok {
				continue
			}
			fromNn := t.blockGraph.From(nnID)
			fromNodes := make([]int64, 0, len(fromNn))
			for _, to := range fromNn {
				toID := to.ID()
				fromNodes = append(fromNodes, toID)
				if _, ok := reachable[toID]; !ok {
					nodStack = append(nodStack, to)
				}
			}
			reachableEdges += len(fromNodes)
			reachable[nn.ID()] = transactionReachableNode{
				from:       fromNodes,
				encodeDone: make(chan struct{}),
			}
		}
		trace.Logf(ctx, "hydra/block/transaction/write-at-root/reachable", "nodes=%d edges=%d", len(reachable), reachableEdges)
	}
	subtask.End()

	// Order encodes after their referenced blocks and shared marshal aliases.
	t.addMarshalAliasWaits(reachable)
	_, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/topo-sort")
	nods, err := SortBlockGraph(t.blockGraph)
	subtask.End()
	if err != nil {
		return nil, nil, err
	}
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/topo", "nodes=%d", len(nods))

	// Use the transaction's resolved hash type for every new block reference.
	hashType := t.putOpts.GetHashType()

	// A single reachable node has no parallel work. Run its encode and put on
	// the caller; larger graphs use the existing bounded worker queues.
	var encodeQueue, writeQueue *conc.ConcurrentQueue
	if len(reachable) > 1 {
		encodeQueue = conc.NewConcurrentQueue(maxEncodeConcurrency)
		writeQueue = conc.NewConcurrentQueue(maxWriteConcurrency)
	}

	// Workers mutate transaction handles while the caller owns t.mtx. If an
	// error makes WaitIdle return early, stop and join every worker before
	// deferred transaction cleanup or unlock can run.
	defer func() {
		subCtxCancel()
		if encodeQueue != nil {
			waitCtx := context.WithoutCancel(ctx)
			_ = encodeQueue.WaitIdle(waitCtx, nil)
			_ = writeQueue.WaitIdle(waitCtx, nil)
		}
	}()

	// Serialize parent updates and retain the first worker failure.
	var mtx sync.Mutex
	errCh := make(chan error, 1)
	handleErr := func(err error) {
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}

	// Retain unreachable handles until every encoder has stopped using them.
	var unreachableNodes []*handle
	if clearTree && subRoot == nil {
		_, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/collect-unreachable")
		for _, v := range slices.Backward(nods) {
			nod := v
			nodID := nod.ID()
			bn, ok := nod.(*handle)
			if !ok || bn == nil {
				continue
			}
			if _, blkReachable := reachable[nodID]; !blkReachable {
				unreachableNodes = append(unreachableNodes, bn)
			}
		}
		trace.Logf(ctx, "hydra/block/transaction/write-at-root/unreachable", "nodes=%d", len(unreachableNodes))
		subtask.End()
	}

	// Encode children before parents, propagating each new reference upward.
	var dirtyNodes int
	var encodedBlocks atomic.Int64
	var putBlocks atomic.Int64
	_, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/schedule-workers")
	for _, v := range slices.Backward(nods) {
		nod := v
		nodID := nod.ID()
		bn, ok := nod.(*handle)
		if !ok || bn == nil {
			continue
		}

		// skip if not reachable from the write root
		reachableNod, blkReachable := reachable[nodID]
		if !blkReachable {
			// we can skip closing encodeDone here since nobody waits on this node.
			continue
		}

		// A non-dirty node still anchors block memory that a dirty
		// descendant mutates during this write: a child's encode applies
		// its computed ref into the parent block via ApplyBlockRef /
		// ApplySubBlock, and an FSNode's Dirent entries are the same
		// *Dirent objects exposed as sub-blocks. The node may be non-dirty
		// because the sub-block handle was materialized after markDirty ran,
		// so dirtiness never propagated through it. Its encodeDone must not
		// close until its subtree finishes, or a parent marshaling this
		// node's shared block races the descendant's ref application
		// (concurrent SizeVT / MarshalToSizedBufferVT on one *Dirent). Gate
		// the close on the subtree off the bounded encode pool so a pure
		// waiter never starves an encode worker.
		if !bn.dirty {
			go func() {
				defer close(reachableNod.encodeDone)
				for _, childID := range reachableNod.from {
					select {
					case <-ctx.Done():
						return
					case <-reachable[childID].encodeDone:
					}
				}
			}()
			continue
		}
		dirtyNodes++

		// Use the same encoder for inline leaves and queued graph nodes.
		encode := func() {
			// Signal completion even when a dependency or hook fails.
			defer close(reachableNod.encodeDone)
			for _, nodID := range reachableNod.from {
				rnod := reachable[nodID]
				select {
				case <-ctx.Done():
					handleErr(context.Canceled)
					return
				case <-rnod.encodeDone:
				}
			}

			// Encode the final hooked block and retain its immutable reference.
			blkRef := bn.ref
			if bn.blk != nil {
				blkRef = nil
				bnpw, bnpwOk := bn.blk.(BlockWithPreWriteHook)
				if bnpwOk {
					if err := bnpw.BlockPreWriteHook(); err != nil {
						handleErr(err)
						return
					}
				}
				if bn.blkPreWrite != nil {
					if err := bn.blkPreWrite(bn.blk); err != nil {
						handleErr(err)
						return
					}
				}
				if !bn.isSubBlock {
					bk, err := CastToBlock(bn.blk)
					if err != nil {
						handleErr(err)
						return
					}
					dat, err := bk.MarshalBlock()
					if err != nil {
						handleErr(err)
						return
					}
					encodedBlocks.Add(1)

					// Empty blocks retain the nil reference without a storage write.
					if len(dat) != 0 {
						if t.xfrm != nil {
							dat, err = t.xfrm.EncodeBlock(dat)
							if err != nil {
								handleErr(err)
								return
							}
						}
						datHash, err := hash.Sum(hashType, dat)
						if err != nil {
							handleErr(err)
							return
						}
						blkRef = NewBlockRef(datHash)
						putOpts := t.putOpts.CloneVT()
						putOpts.HashType = hashType
						putOpts.ForceBlockRef = blkRef

						// Extract block refs before enqueuing write, since
						// bn.blk may be cleared by clearTree after this point.
						putOpts.Refs, err = ExtractBlockRefs(bn.blk)
						if err != nil {
							handleErr(err)
							return
						}

						// Store content without a redundant existence probe.
						put := func() {
							writeCtx, writeTask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/put-block")
							putBlocks.Add(1)
							_, _, err := buffered.putBlock(writeCtx, dat, putOpts, false)
							writeTask.End()
							if err != nil {
								handleErr(err)
								return
							}
						}
						if writeQueue == nil {
							put()
						} else {
							writeQueue.Enqueue(put)
						}
					}
				}
				bn.ref = blkRef
			}

			// Release written cursor data when the caller requested clearing.
			bn.dirty = false
			if clearTree {
				bn.refHandles = nil
				bn.blkPreWrite = nil
			}

			// Release the parent-update lock on application errors as well as success.
			mtx.Lock()
			defer mtx.Unlock()
			for _, ref := range bn.parents {
				sblk := ref.src.blk
				switch {
				case !bn.isSubBlock:
					if clearTree {
						bn.blk = nil // retain root block only
					}
					sblkWithRefs, _ := sblk.(BlockWithRefs)
					if sblkWithRefs != nil {
						if err := sblkWithRefs.ApplyBlockRef(
							ref.id,
							blkRef,
						); err != nil {
							handleErr(err)
							return
						}
					}
				default:
					subBlk, ok := bn.blk.(SubBlock)
					if !ok {
						handleErr(ErrNotSubBlock)
						return
					}
					sblkWithSub, _ := sblk.(BlockWithSubBlocks)
					if sblkWithSub != nil {
						if err := sblkWithSub.ApplySubBlock(
							ref.id,
							subBlk,
						); err != nil {
							handleErr(err)
							return
						}
					}
				}
				if clearTree && ref.src.refHandles != nil {
					delete(ref.src.refHandles, ref.id)
				}
			}
		}

		// A leaf finishes here; graph nodes keep their existing queue lifetime.
		if encodeQueue == nil {
			encode()
			continue
		}
		encodeQueue.Enqueue(encode)
	}
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/dirty", "nodes=%d reachable=%d", dirtyNodes, len(reachable))
	subtask.End()

	// Join scheduled encoders before joining the writes they submitted.
	if encodeQueue != nil {
		taskCtx, subtask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/wait-encode")
		err = encodeQueue.WaitIdle(taskCtx, errCh)
		subtask.End()
		if err != nil {
			return nil, nil, err
		}

		// Encoders can no longer submit new writes.
		taskCtx, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/wait-write")
		err = writeQueue.WaitIdle(taskCtx, errCh)
		subtask.End()
		if err != nil {
			return nil, nil, err
		}
	}

	// Drain only this write's coalescer; borrowed buffers belong to the caller.
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/write-shape", "encoded_blocks=%d put_blocks=%d", encodedBlocks.Load(), putBlocks.Load())
	if buffered != nil && drainBuffered {
		taskCtx, subtask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/drain-write-store")
		buffered.logPendingShape(taskCtx, "hydra/block/transaction/write-at-root/drain-write-store/before")
		err = buffered.drainAll(taskCtx)
		buffered.logPendingShape(taskCtx, "hydra/block/transaction/write-at-root/drain-write-store/after")
		subtask.End()
		if err != nil {
			return nil, nil, err
		}
		if buffered == staged {
			t.stagedStore.CompareAndSwap(staged, nil)
		}
	}

	// Report cancellation or a queued failure before exposing the new root.
	select {
	case <-ctx.Done():
		return nil, nil, context.Canceled
	case err := <-errCh:
		return nil, nil, err
	default:
	}

	// Remove unreachable nodes only after all workers have settled.
	if clearTree && subRoot == nil {
		_, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/cleanup-unreachable")
		for _, bn := range unreachableNodes {
			bn.blk = nil
			bn.ref = nil
			t.blockGraph.RemoveNode(bn.ID())
		}
		subtask.End()
	}

	// The deferred cursor cleanup supplies the returned root cursor.
	return writeRoot.ref, nil, nil
}

// clearData resets the cursor graph under mtx, retaining the root handle.
func (t *Transaction) clearData() {
	t.dirty = false
	t.root.dirty = false
	t.root.refHandles = nil
	t.blockGraph = NewBlockGraph()
	rn := t.blockGraph.NewNode()
	t.root.Node = rn
	t.blockGraph.AddNode(t.root)
}

// addMarshalAliasWaits makes each node's marshal wait for the marshals of
// any alias-linked nodes it references, so alias identities are written
// before the blocks that point at them.
func (t *Transaction) addMarshalAliasWaits(reachable map[int64]transactionReachableNode) {
	// Index handles sharing the same marshal identity.
	if t == nil || t.blockGraph == nil {
		return
	}
	handlesByBlock := make(map[*AliasIdentityToken][]int64, len(reachable))
	for nodeID := range reachable {
		h, _ := t.blockGraph.Node(nodeID).(*handle)
		if h == nil {
			continue
		}
		identity := blockAliasIdentity(h.blk)
		if identity == nil {
			continue
		}
		handlesByBlock[identity] = append(handlesByBlock[identity], nodeID)
	}

	// Add alias dependencies to each reachable block's existing child waits.
	for nodeID := range reachable {
		h, _ := t.blockGraph.Node(nodeID).(*handle)
		if h == nil || h.isSubBlock || h.blk == nil {
			continue
		}
		waitSet := make(map[int64]struct{}, len(reachable[nodeID].from))
		for _, childID := range reachable[nodeID].from {
			waitSet[childID] = struct{}{}
		}
		walkMarshalAliasSubBlocks(h.blk, make(map[*AliasIdentityToken]struct{}), func(subBlock any) {
			identity := blockAliasIdentity(subBlock)
			if identity == nil {
				return
			}
			for _, aliasID := range handlesByBlock[identity] {
				if aliasID == nodeID {
					continue
				}
				if _, ok := reachable[aliasID]; !ok {
					continue
				}
				waitSet[aliasID] = struct{}{}
			}
		})
		next := reachable[nodeID]
		if len(waitSet) == len(next.from) {
			continue
		}
		next.from = next.from[:0]
		for waitID := range waitSet {
			next.from = append(next.from, waitID)
		}
		slices.Sort(next.from)
		reachable[nodeID] = next
	}
}

// blockAliasIdentity returns the alias identity token of a block value,
// or nil.
func blockAliasIdentity(v any) *AliasIdentityToken {
	if v == nil {
		return nil
	}
	withIdentity, ok := v.(BlockWithAliasIdentity)
	if !ok {
		return nil
	}
	return withIdentity.BlockAliasIdentity()
}

// walkMarshalAliasSubBlocks visits every sub-block carrying an alias
// identity exactly once.
func walkMarshalAliasSubBlocks(v any, seen map[*AliasIdentityToken]struct{}, visit func(any)) {
	identity := blockAliasIdentity(v)
	if identity != nil {
		if _, ok := seen[identity]; ok {
			return
		}
		seen[identity] = struct{}{}
	}
	withSubBlocks, ok := v.(BlockWithSubBlocks)
	if !ok {
		return
	}
	for _, subBlock := range withSubBlocks.GetSubBlocks() {
		if subBlock == nil || subBlock.IsNil() {
			continue
		}
		visit(subBlock)
		walkMarshalAliasSubBlocks(subBlock, seen, visit)
	}
}

// cloneDetached copies the transaction for use as a detached tx.
func (t *Transaction) cloneDetached(nroot *handle) *Transaction {
	// Detached transactions retain storage policy but own their cursor graph.
	if t == nil {
		return nil
	}
	nt := &Transaction{
		store:         t.store,
		xfrm:          t.xfrm,
		root:          nroot,
		blockGraph:    NewBlockGraph(),
		putOpts:       t.putOpts,
		decodedBlocks: t.decodedBlocks,
	}
	nt.root.Node = nt.blockGraph.NewNode()
	nt.blockGraph.AddNode(nt.root)
	return nt
}
