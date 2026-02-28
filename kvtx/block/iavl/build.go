package kvtx_block_iavl

import (
	"iter"

	"github.com/aperturerobotics/hydra/block"
)

// BuildTree builds a balanced IAVL tree bottom-up from sorted (key, value ref)
// pairs. The iterator must yield entries in ascending key order.
//
// Each value BlockRef points to an already-written block in storage. The ref is
// set directly on the leaf node's ValueRef proto field — no graph edge is
// created for it, since the block is already persisted.
//
// Returns a Transaction containing all tree nodes and a cursor to the root.
// The caller calls Transaction.Write() to persist the tree to storage.
//
// Returns (nil, nil, nil) if the iterator yields no entries.
func BuildTree(
	store block.StoreOps,
	putOpts *block.PutOpts,
	entries iter.Seq2[[]byte, *block.BlockRef],
) (*block.Transaction, *block.Cursor, error) {
	// Collect leaves from iterator.
	type entry struct {
		key []byte
		ref *block.BlockRef
	}
	var leaves []entry
	for k, v := range entries {
		leaves = append(leaves, entry{key: k, ref: v})
	}
	if len(leaves) == 0 {
		return nil, nil, nil
	}

	// Create a transaction for the tree nodes.
	tx, rootCs := block.NewTransaction(store, nil, nil, putOpts)

	// Build leaf layer: one cursor per entry.
	//
	// minKey tracks the leftmost key in each subtree. For internal nodes, the
	// IAVL routing key must be the leftmost key of the RIGHT subtree (the
	// separator), not the right child's node key (which is the rightmost key
	// of the right subtree).
	type nodeEntry struct {
		cursor *block.Cursor
		height uint32
		size   uint64
		key    []byte // the node's IAVL key
		minKey []byte // leftmost key in this subtree
	}

	layer := make([]nodeEntry, len(leaves))
	for i, lf := range leaves {
		var cs *block.Cursor
		if i == 0 {
			cs = rootCs
		} else {
			cs = rootCs.Detach(false)
		}

		nod := &Node{
			Key:      lf.key,
			Size:     1,
			ValueRef: lf.ref,
		}
		cs.ClearAllRefs()
		cs.SetBlock(nod, true)

		layer[i] = nodeEntry{
			cursor: cs,
			height: 0,
			size:   1,
			key:    lf.key,
			minKey: lf.key,
		}
	}

	// Build internal layers bottom-up until one root remains.
	for len(layer) > 1 {
		next := make([]nodeEntry, 0, (len(layer)+1)/2)
		for i := 0; i+1 < len(layer); i += 2 {
			left := layer[i]
			right := layer[i+1]

			h := maxUint32(left.height, right.height) + 1
			s := left.size + right.size

			// The internal node's key is the leftmost key of the right
			// subtree. This is the separator: keys < separator go left,
			// keys >= separator go right.
			parent := rootCs.Detach(false)
			nod := &Node{
				Key:    right.minKey,
				Height: h,
				Size:   s,
			}
			parent.ClearAllRefs()
			parent.SetBlock(nod, true)
			parent.SetRef(5, left.cursor)
			parent.SetRef(6, right.cursor)

			next = append(next, nodeEntry{
				cursor: parent,
				height: h,
				size:   s,
				key:    right.minKey,
				minKey: left.minKey,
			})
		}
		// Odd node promotes to next layer.
		if len(layer)%2 == 1 {
			next = append(next, layer[len(layer)-1])
		}
		layer = next
	}

	root := layer[0]
	if err := tx.SetRoot(root.cursor); err != nil {
		return nil, nil, err
	}

	return tx, root.cursor, nil
}
