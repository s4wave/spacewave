package block

import (
	"errors"
	"sync"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/simple"
	"gonum.org/v1/gonum/graph/topo"
)

// Transaction tracks refs traversed between blocks, batching writes and
// propagating changes through the merkle tree.
//
// A cache is maintained with decoded block data with a defined memory budget.
// When an entry is invalidated from the cache, the data is released but the
// refs structure between the blocks is maintained. This information is used to
// later re-decode the block, apply changes, and push the changes to storage.
//
// The decoded object form of the block can be stored / attached to a block
// handle. If the block is marked as dirty, the object form is acquired, any ref
// changes are applied, the block is marshaled and transformed for storage, and
// queued for flushing to disk.
type Transaction struct {
	// bucket is the bucket handle
	bucket bucket.Bucket
	// root is the root reference
	root *handle
	// mtx guards the object
	mtx sync.Mutex
	// blockGraph is the graph of blocks
	blockGraph *simple.DirectedGraph
	// putOpts are optional put options
	putOpts *bucket.PutOpts
	// dirty indicates anything changed in the transaction
	dirty bool
}

// NewTransaction builds a new transaction with a root cursor.
func NewTransaction(
	// bkt is the bucket handle
	bkt bucket.Bucket,
	// rootRef is the root reference
	rootRef *cid.BlockRef,
	// putOpts is optional
	putOpts *bucket.PutOpts,
) (*Transaction, *Cursor) {
	t := &Transaction{
		bucket:     bkt,
		root:       &handle{ref: rootRef},
		blockGraph: simple.NewDirectedGraph(),
		putOpts:    putOpts,
	}
	t.root.Node = t.blockGraph.NewNode()
	t.blockGraph.AddNode(t.root)
	cs := newCursor(t, t.root)
	return t, cs
}

// SetRoot sets the root of the transaction to a different position.
func (t *Transaction) SetRoot(cursor *Cursor) error {
	if cursor.t != t {
		return errors.New("cursor block transaction mismatch")
	}
	t.root = cursor.pos
	cursor.pos.parent = nil
	cursor.pos.dirty = true
	t.dirty = true
	return nil
}

// GetBlockGraph returns the internal block graph state.
// Do not modify this, used for analysis.
func (t *Transaction) GetBlockGraph() graph.Graph {
	return t.blockGraph
}

// Write writes the dirty blocks to the store, propagating reference changes up
// the tree. Clears the blocks cache. The final block in the event list will be
// the new root. The new root cursor is set up appropriately and returned.
//
// Note: only the new returned root cursor is valid after a Write()!
func (t *Transaction) Write() (
	res []*bucket_event.Event,
	rcursor *Cursor,
	rerr error,
) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer func() {
		if rerr == nil {
			t.clearData()
			rcursor = newCursor(t, t.root)
		}
	}()

	if !t.dirty {
		return nil, nil, nil
	}

	// Pass 1: cut all subtrees with nil blocks.
	var cutEvents []*bucket_event.Event
	// todo: the list of "cut" blocks is not always reliable.
	pushCut := func(h *handle) {
		var prevRef *cid.BlockRef
		if h.parent != nil && h.parent.src != nil {
			prevRef = h.parent.src.ref
		}
		cutEvents = append(cutEvents, &bucket_event.Event{
			EventType: bucket_event.EventType_EventType_CUT_BLOCK,
			CutBlock: &bucket_event.CutBlock{
				BlockCommon: &bucket_event.BlockCommon{
					BlockRef: h.ref,
				},
				PrevRef: prevRef,
			},
		})
	}
	it := t.blockGraph.Nodes()
	for it.Next() {
		nod := it.Node().(*handle)
		if !nod.dirty {
			continue
		}
		// check if this block should be unconditionally removed:
		// (this was commented out to allow for unsetting fields)
		/*
			if nod.blk == nil && nod.parent.src.r {
					if !nod.ref.GetEmpty() && !nod.isSubBlock {
						pushCut(nod)
					}
				t.blockGraph.RemoveNode(nod.ID())
				continue
			}
		*/
		if nod.isSubBlock {
			// check if the parent sub-block has changed
			fromID := nod.parent.From().ID()
			toID := nod.ID()
			if !t.blockGraph.HasEdgeBetween(fromID, toID) {
				continue
			}
			// if the parent block's sub-block field was set to something else,
			// cut this entire sub-graph
			src := nod.parent.src
			refID := nod.parent.id
			bsb, _ := src.blk.(BlockWithSubBlocks)
			if bsb == nil {
				t.blockGraph.RemoveEdge(fromID, toID)
				continue
			}
			subBlockCtor := bsb.GetSubBlockCtor(refID)
			if subBlockCtor == nil {
				t.blockGraph.RemoveEdge(fromID, toID)
				continue
			}
			/* Note: sometimes we return unique pointers from ctor
			subBlkObj := subBlockCtor(false)
			if subBlkObj != nod.blk {
				t.blockGraph.RemoveEdge(fromID, toID)
			}
			*/
		}
	}

	// Pass 2 [partial]: mark blocks not reachable from root.
	// spt := path.DijkstraFrom(t.root.nod, t.blockGraph)
	reachable := make(map[int64]struct{})
	reachable[t.root.ID()] = struct{}{}
	{
		nodStack := []graph.Node{t.root}
		for len(nodStack) != 0 {
			nn := nodStack[len(nodStack)-1]
			nodStack = nodStack[:len(nodStack)-1]
			fromNn := t.blockGraph.From(nn.ID())
			for fromNn.Next() {
				to := fromNn.Node()
				if _, ok := reachable[to.ID()]; !ok {
					reachable[to.ID()] = struct{}{}
					nodStack = append(nodStack, to)
				}
			}
		}
	}

	// Pass 3. topological sort
	nods, err := topo.Sort(t.blockGraph)
	if err != nil {
		return nil, nil, err
	}

	for ni := len(nods) - 1; ni >= 0; ni-- {
		nod := nods[ni]
		nodID := nod.ID()
		bn, ok := nod.(*handle)
		if !ok || bn == nil {
			continue
		}

		_, blkReachable := reachable[nodID]
		if !blkReachable {
			if !bn.ref.GetEmpty() {
				pushCut(bn)
				bn.blk = nil
				bn.ref = nil
			}
			continue
		}

		if !bn.dirty {
			continue
		}

		bn.dirty = false
		var blkRef *cid.BlockRef
		if bn.blk != nil {
			bnpw, bnpwOk := bn.blk.(BlockWithPreWriteHook)
			if bnpwOk {
				if err := bnpw.BlockPreWriteHook(); err != nil {
					return nil, nil, err
				}
			}

			if bn.blkPreWrite != nil {
				if err := bn.blkPreWrite(bn.blk); err != nil {
					return nil, nil, err
				}
			}

			if !bn.isSubBlock {
				bk, err := castToBlock(bn.blk)
				if err != nil {
					return res, nil, err
				}

				dat, err := bk.MarshalBlock()
				if err != nil {
					return res, nil, err
				}

				be, err := t.bucket.PutBlock(dat, t.putOpts)
				if err != nil {
					return res, nil, err
				}
				res = append(res, &bucket_event.Event{
					EventType: bucket_event.EventType_EventType_PUT_BLOCK,
					PutBlock:  be,
				})
				blkRef = be.GetBlockCommon().GetBlockRef()
			}
		} else {
			blkRef = bn.ref
		}

		bn.ref = blkRef
		bn.refHandles = nil
		bn.blkPreWrite = nil
		if ref := bn.parent; ref != nil {
			sblk := ref.src.blk
			if !bn.isSubBlock {
				bn.blk = nil // retain root block only
				sblkWithRefs, _ := sblk.(BlockWithRefs)
				if sblkWithRefs != nil {
					if err := sblkWithRefs.ApplyBlockRef(
						ref.id,
						blkRef,
					); err != nil {
						return res, nil, err
					}
				}
			} else {
				sblkWithSub, _ := sblk.(BlockWithSubBlocks)
				if sblkWithSub != nil {
					if err := sblkWithSub.ApplySubBlock(
						ref.id,
						bn.blk,
					); err != nil {
						return res, nil, err
					}
				}
			}
			if ref.src.refHandles != nil {
				delete(ref.src.refHandles, ref.id)
			}
		}
	}

	if len(cutEvents) != 0 {
		cutEvents = append(cutEvents, res...)
		res = cutEvents
	}

	// build new root cursor
	return res, newCursor(t, t.root), nil
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
