package block

import (
	"context"
	"runtime"
	"sync"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/util/conc"
	simple "github.com/paralin/gonum-graph-simple"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/topo"
)

// maxWriteConcurrency is the maximum concurrency for PutBlock calls.
// NOTE: this may be configurable or dynamic in future.
var maxWriteConcurrency = runtime.GOMAXPROCS(0)

// maxEncodeConcurrency is the maximum concurrency for hashing & marshaling blocks.
// NOTE: this may be configurable or dynamic in future.
var maxEncodeConcurrency = maxWriteConcurrency

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
	blockGraph *simple.DirectedGraph
	// putOpts are put options (hashType is always filled with a value)
	putOpts *PutOpts
	// dirty indicates anything changed in the transaction
	dirty bool
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
		blockGraph: simple.NewDirectedGraph(),
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

// SetRoot sets the root of the transaction to a different position.
// Clears all parent blocks from the new root.
func (t *Transaction) SetRoot(cursor *Cursor) error {
	if t == nil {
		return nil
	} else if cursor.t != nil && cursor.t != t {
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
	if t == nil {
		return nil, nil, tx.ErrNotWrite
	}

	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer func() {
		if clearTree {
			t.clearData()
		}
		if rcursor == nil {
			rcursor = newCursor(t, t.root, nil)
		}
	}()

	if !t.dirty {
		return t.root.ref, nil, nil
	}

	// create a sub-context
	ctx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	type reachableNode struct {
		// from is the list of nodes that we can reach from this node
		// (child nodes)
		from []int64
		// encodeDone is closed when encoding this node is done.
		encodeDone chan struct{}
	}

	// mark blocks reachable from root. we will drop (cut) unreachable blocks.
	// create a channel for each that is closed when we've written the block.
	reachable := make(map[int64]reachableNode, 1)
	{
		nodStack := []graph.Node{t.root}
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
			reachable[nn.ID()] = reachableNode{
				from:       fromNodes,
				encodeDone: make(chan struct{}),
			}
		}
	}

	// topological sort to determine dependencies (references, etc).
	nods, err := topo.Sort(t.blockGraph)
	if err != nil {
		return nil, nil, err
	}

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

	// collect unreachable nodes for later cleanup
	var unreachableNodes []*handle
	if clearTree {
		for ni := len(nods) - 1; ni >= 0; ni-- {
			nod := nods[ni]
			nodID := nod.ID()
			bn, ok := nod.(*handle)
			if !ok || bn == nil {
				continue
			}
			if _, blkReachable := reachable[nodID]; !blkReachable {
				unreachableNodes = append(unreachableNodes, bn)
			}
		}
	}

	// process the topological sort to schedule write jobs
	// determine if the blocks are dirty or not before scheduling writing.
	// nods is sorted by [root, ..., furthest child]
	//
	// concurrently marshal + transform + hash blocks.
	// after hashing: write the updated BlockRef to parent blocks.
	// push the marshalled blocks to the block write queue.
	for ni := len(nods) - 1; ni >= 0; ni-- {
		nod := nods[ni]
		nodID := nod.ID()
		bn, ok := nod.(*handle)
		if !ok || bn == nil {
			continue
		}

		// skip if not reachable
		reachableNod, blkReachable := reachable[nodID]
		if !blkReachable {
			// we can skip closing encodeDone here since nobody waits on this node.
			continue
		}

		// skip if not dirty
		if !bn.dirty {
			close(reachableNod.encodeDone)
			continue
		}

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

						writeQueue.Enqueue(func() {
							// ensure that the wrote ref == the expected.
							wroteRef, _, err := t.store.PutBlock(ctx, dat, putOpts)
							if err == nil && !wroteRef.EqualsRef(blkRef) {
								err = errors.Errorf("wrote block ref %s != expected %s", wroteRef.MarshalString(), blkRef.MarshalString())
							}
							if err != nil {
								handleErr(err)
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

	// wait for all tasks to complete
	if err := encodeQueue.WaitIdle(ctx, errCh); err != nil {
		return nil, nil, err
	}
	if err := writeQueue.WaitIdle(ctx, errCh); err != nil {
		return nil, nil, err
	}

	// check there are no remaining queued errors
	select {
	case <-ctx.Done():
		return nil, nil, context.Canceled
	case err := <-errCh:
		return nil, nil, err
	default:
	}

	// clean up unreachable nodes after all workers complete
	if clearTree {
		for _, bn := range unreachableNodes {
			bn.blk = nil
			bn.ref = nil
			t.blockGraph.RemoveNode(bn.ID())
		}
	}

	// note: defer func builds new root cursor (second field)
	return t.root.ref, nil, nil
}

// clearData clears all data. expects mtx to be locked by caller.
// the root remains, and the root cursor will still be valid.
func (t *Transaction) clearData() {
	t.dirty = false
	t.root.dirty = false
	t.root.refHandles = nil
	t.blockGraph = simple.NewDirectedGraph()
	rn := t.blockGraph.NewNode()
	t.root.Node = rn
	t.blockGraph.AddNode(t.root)
}

// cloneDetached copies the transaction for use as a detached tx.
func (t *Transaction) cloneDetached(nroot *handle) *Transaction {
	if t == nil {
		return nil
	}
	nt := &Transaction{
		store:      t.store,
		xfrm:       t.xfrm,
		root:       nroot,
		blockGraph: simple.NewDirectedGraph(),
		putOpts:    t.putOpts,
	}
	nt.root.Node = nt.blockGraph.NewNode()
	nt.blockGraph.AddNode(nt.root)
	return nt
}
