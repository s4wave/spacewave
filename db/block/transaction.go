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
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/topo"
)

// maxWriteConcurrency is the maximum concurrency for PutBlock calls.
var maxWriteConcurrency = runtime.GOMAXPROCS(0)

// maxEncodeConcurrency is the maximum concurrency for hashing & marshaling blocks.
var maxEncodeConcurrency = maxWriteConcurrency

type transactionReachableNode struct {
	// from is the list of nodes that we can reach from this node
	// (child nodes)
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
	// store is the block store handle
	store StoreOps
	// xfrm is an optional block transformer
	xfrm Transformer
	// root is the root reference
	root *handle
	// mtx guards the object
	mtx sync.Mutex
	// blockGraph is the graph of blocks
	blockGraph *directedGraph
	// putOpts are put options (hashType is always filled with a value)
	putOpts *PutOpts
	// dirty indicates anything changed in the transaction
	dirty bool
	// bufferedStoreSettings overrides the default BufferedStore settings used
	// inside WriteAtRoot. nil uses the defaults.
	bufferedStoreSettings *BufferedStoreSettings
	// decodedBlocks is borrowed from the owning object lifecycle.
	decodedBlocks *DecodedBlockCache
}

// NewTransaction builds a new transaction with a root cursor.
func NewTransaction(
	// store is the block store
	store StoreOps,
	// transformer is an optional block transformer
	transformer Transformer,
	// rootRef is the root reference
	rootRef *BlockRef,
	// putOpts is optional
	putOpts *PutOpts,
) (*Transaction, *Cursor) {
	if putOpts == nil {
		putOpts = &PutOpts{}
	} else {
		putOpts = putOpts.CloneVT()
		putOpts.ForceBlockRef = nil
	}

	// determine which hash type to use
	hashType := putOpts.GetHashType()
	if hashType == 0 && store != nil {
		hashType = store.GetHashType()
	}
	if hashType == 0 {
		hashType = DefaultHashType
	}
	putOpts.HashType = hashType

	t := &Transaction{
		store:      store,
		xfrm:       transformer,
		root:       &handle{ref: rootRef},
		blockGraph: newDirectedGraph(),
		putOpts:    putOpts,
	}
	t.root.Node = t.blockGraph.NewNode()
	t.blockGraph.AddNode(t.root)
	cs := newCursor(t, t.root, nil)
	return t, cs
}

// GetBlockGraph returns a handle to the internal block graph state.
// Do not modify this, used for analysis.
func (t *Transaction) GetBlockGraph() graph.Graph {
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

// SetDecodedBlockCache sets the lifecycle-owned decoded-block cache borrowed by this transaction.
func (t *Transaction) SetDecodedBlockCache(cache *DecodedBlockCache) {
	if t == nil {
		return
	}
	t.mtx.Lock()
	t.decodedBlocks = cache
	t.mtx.Unlock()
}

// SetBufferedStoreSettings overrides the BufferedStore settings used to wrap
// the write store inside WriteAtRoot. Pass nil to reset to defaults. This must
// be called before Write/WriteAtRoot begins committing for the override to
// take effect on that commit.
func (t *Transaction) SetBufferedStoreSettings(s *BufferedStoreSettings) {
	if t == nil {
		return
	}
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
	if t == nil {
		return nil
	}
	if cursor.t != nil && cursor.t != t {
		return errors.New("cursor block transaction mismatch")
	}
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
	ctx, task := trace.NewTask(ctx, "hydra/block/transaction/write-at-root")
	defer task.End()

	if t == nil {
		return nil, nil, tx.ErrNotWrite
	}

	// determine the write root
	writeRoot := t.root
	if subRoot != nil {
		if subRoot.t != nil && subRoot.t != t {
			return nil, nil, errors.New("cursor block transaction mismatch")
		}
		writeRoot = subRoot.pos
	}

	// deferFlush batches GC ref-graph flushes for dirty writes (see below).
	// registered BEFORE t.mtx.Lock so EndDeferFlush runs AFTER
	// t.mtx.Unlock in LIFO order. The outermost EndDeferFlush calls the GC
	// FlushPending, which must run after the cursor mutex is released because
	// the RefGraph may share it. Uses the parent ctx because subCtxCancel runs
	// first in LIFO.
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

	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer func() {
		// only clear the full tree and reset root when writing from the tx root
		if clearTree && subRoot == nil {
			t.clearData()
		}
		if rcursor == nil {
			rcursor = newCursor(t, writeRoot, nil)
		}
	}()

	if !t.dirty {
		return writeRoot.ref, nil, nil
	}

	// buffered is the per-write coalescer wrapping the write store. It is held
	// as the concrete type so the in-lock drain below can drain it without
	// applying a durability fence (Sync), which would needlessly fence the
	// inner store on every sub-tree write.
	var buffered *BufferedStore
	if writeStore != nil {
		buffered = NewBufferedStoreWithSettings(ctx, writeStore, t.bufferedStoreSettings)
		writeStore = buffered
	}

	// begin deferred GC flushing.
	// only activated for dirty transactions so a non-dirty WriteAtRoot
	// never touches the shared flush counter or flushes another
	// transaction's buffered refs.
	if writeStore != nil {
		deferFlushActive = true
		BeginDeferFlush(writeStore)
	}

	// create a sub-context
	ctx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// mark blocks reachable from the write root.
	// when writing the full tree, unreachable blocks are dropped (cut).
	reachable := make(map[int64]transactionReachableNode, 1)
	_, subtask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/mark-reachable")
	{
		var reachableEdges int
		nodStack := []graph.Node{writeRoot}
		for len(nodStack) != 0 {
			nn := nodStack[len(nodStack)-1]
			nodStack = nodStack[:len(nodStack)-1]
			nnID := nn.ID()
			if _, ok := reachable[nnID]; ok {
				continue
			}
			fromNn := t.blockGraph.From(nnID)
			fromNnLen := max(fromNn.Len(), 0)
			fromNodes := make([]int64, 0, fromNnLen)
			for fromNn.Next() {
				to := fromNn.Node()
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

	t.addMarshalAliasWaits(reachable)

	// topological sort to determine dependencies (references, etc).
	_, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/topo-sort")
	nods, err := topo.Sort(t.blockGraph)
	subtask.End()
	if err != nil {
		return nil, nil, err
	}
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/topo", "nodes=%d", len(nods))

	// hashType is the hash type we will use to build BlockRefs
	hashType := t.putOpts.GetHashType()

	// encodeQueue is the job queue to encode data.
	encodeQueue := conc.NewConcurrentQueue(maxEncodeConcurrency)
	// writeQueue is the job queue to write blocks to the store.
	writeQueue := conc.NewConcurrentQueue(maxWriteConcurrency)

	// mtx is locked while updating parents, as this may result in concurrent map writes otherwise.
	var mtx sync.Mutex
	// errCh is pushed to if there are any errors
	errCh := make(chan error, 1)
	handleErr := func(err error) {
		if err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}

	// collect unreachable nodes for later cleanup (only for full-tree writes)
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

	// process the topological sort to schedule write jobs
	// determine if the blocks are dirty or not before scheduling writing.
	// nods is sorted by [root, ..., furthest child]
	//
	// concurrently marshal + transform + hash blocks.
	// after hashing: write the updated BlockRef to parent blocks.
	// push the marshalled blocks to the block write queue.
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

		encodeQueue.Enqueue(func() {
			defer close(reachableNod.encodeDone)

			// wait for all blocks downstream of this one to finish writing
			for _, nodID := range reachableNod.from {
				rnod := reachable[nodID]
				select {
				case <-ctx.Done():
					handleErr(context.Canceled)
					return
				case <-rnod.encodeDone:
				}
			}

			// encode this node & determine block ref for it
			var blkRef *BlockRef
			if bn.blk != nil {
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

					// use an empty BlockRef to represent empty blocks
					if len(dat) == 0 {
						blkRef = nil // NewBlockRef(nil)
					} else {
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

						writeQueue.Enqueue(func() {
							writeCtx, writeTask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/put-block")
							putBlocks.Add(1)
							// ensure that the wrote ref == the expected.
							wroteRef, _, err := writeStore.PutBlock(writeCtx, dat, putOpts)
							writeTask.End()
							if err == nil && !wroteRef.EqualsRef(blkRef) {
								err = errors.Errorf("wrote block ref %s != expected %s", wroteRef.MarshalString(), blkRef.MarshalString())
							}
							if err != nil {
								handleErr(err)
								return
							}
						})
					}
				}
				bn.ref = blkRef
			} else {
				blkRef = bn.ref
			}

			bn.dirty = false
			if clearTree {
				bn.refHandles = nil
				bn.blkPreWrite = nil
				// TODO: delete node from graph here?
			}

			// lock while processing parents
			mtx.Lock()
			for _, ref := range bn.parents {
				sblk := ref.src.blk
				if !bn.isSubBlock {
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
				} else {
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
			mtx.Unlock()
		})
	}
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/dirty", "nodes=%d reachable=%d", dirtyNodes, len(reachable))
	subtask.End()

	// wait for all tasks to complete
	taskCtx, subtask := trace.NewTask(ctx, "hydra/block/transaction/write-at-root/wait-encode")
	err = encodeQueue.WaitIdle(taskCtx, errCh)
	subtask.End()
	if err != nil {
		return nil, nil, err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/wait-write")
	err = writeQueue.WaitIdle(taskCtx, errCh)
	subtask.End()
	if err != nil {
		return nil, nil, err
	}
	trace.Logf(ctx, "hydra/block/transaction/write-at-root/write-shape", "encoded_blocks=%d put_blocks=%d", encodedBlocks.Load(), putBlocks.Load())
	if buffered != nil {
		taskCtx, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/drain-write-store")
		buffered.logPendingShape(taskCtx, "hydra/block/transaction/write-at-root/drain-write-store/before")
		err = buffered.drainAll(taskCtx)
		buffered.logPendingShape(taskCtx, "hydra/block/transaction/write-at-root/drain-write-store/after")
		subtask.End()
		if err != nil {
			return nil, nil, err
		}
	}

	// check there are no remaining queued errors
	select {
	case <-ctx.Done():
		return nil, nil, context.Canceled
	case err := <-errCh:
		return nil, nil, err
	default:
	}

	// clean up unreachable nodes after all workers complete (full-tree writes only)
	if clearTree && subRoot == nil {
		_, subtask = trace.NewTask(ctx, "hydra/block/transaction/write-at-root/cleanup-unreachable")
		for _, bn := range unreachableNodes {
			bn.blk = nil
			bn.ref = nil
			t.blockGraph.RemoveNode(bn.ID())
		}
		subtask.End()
	}

	// note: defer func builds new root cursor (second field)
	return writeRoot.ref, nil, nil
}

// clearData clears all data. expects mtx to be locked by caller.
// the root remains, and the root cursor will still be valid.
func (t *Transaction) clearData() {
	t.dirty = false
	t.root.dirty = false
	t.root.refHandles = nil
	t.blockGraph = newDirectedGraph()
	rn := t.blockGraph.NewNode()
	t.root.Node = rn
	t.blockGraph.AddNode(t.root)
}

func (t *Transaction) addMarshalAliasWaits(reachable map[int64]transactionReachableNode) {
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
	if t == nil {
		return nil
	}
	nt := &Transaction{
		store:         t.store,
		xfrm:          t.xfrm,
		root:          nroot,
		blockGraph:    newDirectedGraph(),
		putOpts:       t.putOpts,
		decodedBlocks: t.decodedBlocks,
	}
	nt.root.Node = nt.blockGraph.NewNode()
	nt.blockGraph.AddNode(nt.root)
	return nt
}
