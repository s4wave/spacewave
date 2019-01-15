package block

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
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
	// blocks are the block references
	blocks map[int64]*handle
	// blockGraph is the graph of blocks
	blockGraph *simple.DirectedGraph
	// putOpts are optional put options
	putOpts *bucket.PutOpts
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
		blocks:     make(map[int64]*handle),
		blockGraph: simple.NewDirectedGraph(),
		putOpts:    putOpts,
	}
	t.root.nod = t.blockGraph.NewNode()
	t.blocks[t.root.nod.ID()] = t.root
	t.blockGraph.AddNode(t.root.nod)
	cs := newCursor(t, t.root)
	return t, cs
}

// Write writes the dirty blocks to the store, propagating reference changes up
// the tree. Clears the blocks cache. The final block in the event list will be
// the new root. The new root cursor is set up appropriately and returned.
func (t *Transaction) Write(rctx context.Context) (
	res []*bucket_event.PutBlock,
	rcursor *Cursor,
	rerr error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()
	t.mtx.Lock()
	defer t.mtx.Unlock()
	defer func() {
		if rerr == nil {
			t.clearData()
			rcursor = newCursor(t, t.root)
		}
	}()

	// concurrently write using the DAG dependency tree
	nods, err := topo.Sort(t.blockGraph)
	if err != nil {
		return nil, nil, err
	}

	for ni := len(nods) - 1; ni >= 0; ni-- {
		nod := nods[ni]
		bn, ok := t.blocks[nod.ID()]
		if !ok || !bn.dirty {
			continue
		}

		bn.dirty = false
		if bn.blk == nil {
			continue
		}

		if bn.blkPreWrite != nil {
			if err := bn.blkPreWrite(bn.blk); err != nil {
				return nil, nil, err
			}
		}

		dat, err := bn.blk.MarshalBlock()
		if err != nil {
			return res, nil, err
		}
		select {
		case <-ctx.Done():
			return res, nil, ctx.Err()
		default:
		}

		be, err := t.bucket.PutBlock(dat, t.putOpts)
		if err != nil {
			return res, nil, err
		}
		res = append(res, be)

		bn.blk = nil
		bn.blkPreWrite = nil
		bn.refHandles = nil
		if ref := bn.parent; ref != nil {
			if sblk := ref.src.blk; sblk != nil {
				if err := sblk.ApplyRef(
					ref.id,
					be.GetBlockCommon().GetBlockRef(),
				); err != nil {
					return res, nil, err
				}
			}
			if ref.src.refHandles != nil {
				delete(ref.src.refHandles, ref.id)
			}
		}
	}

	return res, nil, nil
}

// clearData clears all data. expects mtx to be locked by caller.
// the root remains, and the root cursor will still be valid.
func (t *Transaction) clearData() {
	t.root.dirty = false
	t.root.refHandles = nil
	for k, b := range t.blocks {
		if b != t.root {
			delete(t.blocks, k)
		}
	}
	t.blockGraph = simple.NewDirectedGraph()
	rn := t.blockGraph.NewNode()
	t.blockGraph.AddNode(rn)
	t.root.nod = rn
}
