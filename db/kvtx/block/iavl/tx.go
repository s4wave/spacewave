package kvtx_block_iavl

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/kvtx"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// Tx is an iavl k/v transaction.
type Tx struct {
	write bool
	bcs   *block.Cursor
	root  *Node

	t          *AVLTree
	tx         *block.Transaction
	rel        func()
	commitOnce atomic.Bool

	// rootChangedCb is called if the root cursor changed.
	// may be nil
	rootChangedCb func(*block.Cursor)
}

// NewTx constructs a new IAVL transaction decoupled from the tree, commit and
// discard will be no-op. Note: the root of the tree will change after many set
// operations, it will be necessary to update any references as well.
//
// btx may be nil, if set, will call Write() on it when Commit() is called.
// ctx is used to fetch and unmarshal the node only
func NewTx(
	ctx context.Context,
	bcs *block.Cursor, btx *block.Transaction,
	write bool,
	rootChangedCb func(*block.Cursor),
) (*Tx, error) {
	// Trace construction of the IAVL transaction.
	ctx, task := trace.NewTask(ctx, "hydra/kvtx-block-iavl/new-tx")
	defer task.End()

	// Decode the root Node before exposing the transaction.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/kvtx-block-iavl/new-tx/unmarshal-root")
	rn, err := block.UnmarshalBlock[*Node](taskCtx, bcs, NewNodeBlock)
	subtask.End()
	if err != nil {
		return nil, err
	}
	return &Tx{
		tx:            btx,
		write:         write,
		bcs:           bcs,
		root:          rn,
		rootChangedCb: rootChangedCb,
	}, nil
}

// GetCursor returns the cursor pointing to the root of the tree.
// This cursor may change after write operations.
func (t *Tx) GetCursor() *block.Cursor {
	return t.bcs
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) (cerr error) {
	if t.commitOnce.CompareAndSwap(false, true) {
		if t.write && t.tx != nil {
			br, _, err := t.tx.Write(ctx, true)
			if err != nil {
				cerr = err
			} else {
				t.t.setRootRef(br)
			}
		}
		if t.rel != nil {
			t.rel()
		}
	}
	return
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	if t.commitOnce.CompareAndSwap(false, true) {
		if t.rel != nil {
			t.rel()
		}
	}
}

// Size returns the number of keys in the tree.
func (t *Tx) Size(ctx context.Context) (uint64, error) {
	return t.root.GetSize(), nil
}

// Height returns the height of the tree.
func (t *Tx) Height() uint32 {
	return t.root.GetHeight()
}

// Exists returns whether or not a key exists.
func (t *Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	if t.root.GetSize() == 0 {
		return false, nil
	}
	return t.hasFromNode(ctx, t.bcs, t.root, key)
}

// Get returns the value of the specified key if it exists.
func (t *Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	// Require a key before reading the IAVL tree.
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	// Finish the lookup when the IAVL tree is empty.
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}

	// Find the leaf that holds the requested key.
	bcs, node, err := t.getFromRoot(ctx, key)
	if err != nil || node == nil || bcs == nil {
		return nil, false, err
	}

	// Read the value stored by the matching leaf.
	val, err := t.nodeToValue(ctx, bcs, node)
	if err != nil {
		return nil, true, err
	}

	return val, true, nil
}

// GetBatch returns values for multiple keys.
func (t *Tx) GetBatch(ctx context.Context, keys [][]byte) ([][]byte, []bool, error) {
	// Prepare indexed lookups and result slots for the requested keys.
	values := make([][]byte, len(keys))
	found := make([]bool, len(keys))
	lookups := make([]batchLookup, 0, len(keys))
	for i, key := range keys {
		if len(key) == 0 {
			return nil, nil, kvtx.ErrEmptyKey
		}
		lookups = append(lookups, batchLookup{
			key:   key,
			index: i,
		})
	}

	// Honor cancellation before traversing the IAVL tree.
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	// Finish an empty batch or an empty tree without traversal.
	if t.root.GetSize() == 0 || len(lookups) == 0 {
		return values, found, nil
	}

	// Fill the batch results through the shared tree traversal.
	if err := t.getBatchFromNode(ctx, t.bcs, t.root, lookups, values, found); err != nil {
		return nil, nil, err
	}
	return values, found, nil
}

// GetCursorAtKey returns the cursor at the specified key, if it exists.
// If the key was updated with Set(), points to a Blob.
//
// Returns nil, nil if not found.
func (t *Tx) GetCursorAtKey(ctx context.Context, key []byte) (*block.Cursor, error) {
	// Require a key before locating its value cursor.
	if len(key) == 0 {
		return nil, kvtx.ErrEmptyKey
	}

	// Finish the cursor lookup when the tree is empty.
	if t.root.GetSize() == 0 {
		return nil, nil
	}

	// Locate the leaf whose value cursor is requested.
	bcs, nod, err := t.getFromRoot(ctx, key)
	if err != nil || bcs == nil || nod == nil {
		return nil, err
	}

	// Follow an embedded Blob when the leaf stores one.
	if nod.ValueIsBlob() {
		return bcs.FollowSubBlock(8), nil
	}
	return bcs.FollowRef(7, nod.GetValueRef()), nil
}

// Set sets a key to a value.
// Uses a Blob internally to chunk large data.
func (t *Tx) Set(ctx context.Context, key []byte, val []byte) (err error) {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	// write the blob
	var valueCursor *block.Cursor
	if len(val) != 0 {
		valueCursor = t.bcs.Detach(false)
		valueCursor.ClearAllRefs()
		rdr := bytes.NewReader(val)
		// stores into valueCursor
		_, err = blob.BuildBlob(
			ctx,
			int64(len(val)), rdr,
			valueCursor,
			nil,
		)
		if err != nil {
			return err
		}
	}

	return t.setFromRoot(ctx, key, valueCursor, true)
}

// SetCursorAtKey sets the key to a reference to the object at bcs.
// if bcs == nil, the key is set with a empty block ref.
func (t *Tx) SetCursorAtKey(ctx context.Context, key []byte, bcs *block.Cursor, isBlob bool) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	return t.setFromRoot(ctx, key, bcs, isBlob)
}

// Delete removes a key from the tree
func (t *Tx) Delete(ctx context.Context, key []byte) error {
	// Require a key before deleting from the IAVL tree.
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	// Establish an empty root when the transaction has none.
	if t.root == nil {
		t.root = &Node{}
	}

	// Finish deletion when the IAVL tree is empty.
	if t.root.GetSize() == 0 {
		return nil
	}

	// Remove the key and publish the replacement root.
	_, _, err := t.removeFromRoot(ctx, key)
	return err
}

// ScanPrefix iterates over keys with a prefix.
// Ascending.
func (t *Tx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, val []byte) error) error {
	return t.scanPrefixLeaves(ctx, prefix, func(bcs *block.Cursor, n *Node) error {
		nodValue, err := t.nodeToValue(ctx, bcs, n)
		if err != nil {
			return err
		}
		return cb(n.GetKey(), nodValue)
	})
}

// ScanPrefixKeys iterates over keys with a prefix.
// Ascending.
func (t *Tx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.scanPrefixLeaves(ctx, prefix, func(_ *block.Cursor, n *Node) error {
		return cb(n.GetKey())
	})
}

// scanPrefixLeaves calls cb with each leaf whose key has the prefix, in
// ascending order. The range ends before the prefix successor, or at the end
// of the tree when the prefix has none.
func (t *Tx) scanPrefixLeaves(ctx context.Context, prefix []byte, cb func(*block.Cursor, *Node) error) error {
	if t.root.GetSize() == 0 {
		return nil
	}
	end, _ := kvtx.PrefixSuccessor(prefix)
	return t.scanLeaves(ctx, t.bcs, t.root, prefix, end, cb)
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// Iterates in sorted order, reverse reverses the key iteration.
func (t *Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return t.IterateIavl(ctx, prefix, sort, reverse)
}

// IterateIavl returns the iavl iterator.
func (t *Tx) IterateIavl(ctx context.Context, prefix []byte, sort, reverse bool) *Iterator {
	return NewIterator(ctx, t, prefix, sort, reverse)
}

// BlockIterate returns the block iterator.
func (t *Tx) BlockIterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.BlockIterator {
	return NewIterator(ctx, t, prefix, sort, reverse)
}

// DeleteCursorAtKey deletes the key and returns the cursor to the value.
// returns nil, nil if not found.
func (t *Tx) DeleteCursorAtKey(ctx context.Context, key []byte) (*block.Cursor, error) {
	// Require a key before removing its value cursor.
	if len(key) == 0 {
		return nil, kvtx.ErrEmptyKey
	}

	// Establish an empty root when the transaction has none.
	if t.root == nil {
		t.root = &Node{}
	}

	// Finish cursor removal when the tree is empty.
	if t.root.GetSize() == 0 {
		return nil, nil
	}

	// Remove the leaf while retaining its value cursor.
	removedNodCursor, removedNod, err := t.removeFromRoot(ctx, key)
	if err != nil {
		return nil, err
	}

	// Follow the removed leaf's embedded Blob when present.
	if removedNod.ValueIsBlob() {
		return removedNodCursor.FollowSubBlock(8), nil
	}
	return removedNodCursor.FollowRef(7, removedNod.GetValueRef()), nil
}

// GetAndDelete removes a key from the tree returning a value.
func (t *Tx) GetAndDelete(ctx context.Context, key []byte) (_ []byte, _ bool, err error) {
	// Require a key before removing its value.
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	// Establish an empty root when the transaction has none.
	if t.root == nil {
		t.root = &Node{}
	}

	// Finish value removal when the tree is empty.
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}

	// Remove the matching leaf and retain it for value decoding.
	removedBcs, removedNod, err := t.removeFromRoot(ctx, key)
	if err != nil || removedBcs == nil {
		return nil, false, err
	}

	// Decode the value retained by the removed leaf.
	val, err := t.nodeToValue(ctx, removedBcs, removedNod)
	return val, true, err
}

// removeFromRoot removes the key from the root and returns the cursor to the removed node.
func (t *Tx) removeFromRoot(ctx context.Context, key []byte) (*block.Cursor, *Node, error) {
	// Remove the leaf from the current root subtree.
	nextCs, _, removedNodCursor, removedNod, err := t.removeFromNode(ctx, t.bcs, t.root, key)
	if err != nil || removedNod == nil {
		return nil, nil, err
	}

	// Load the surviving root or clear the exhausted tree.
	var nextNod *Node
	if nextCs == nil {
		nextCs = t.bcs
		nextNod = &Node{}
		nextCs.SetBlock(nextNod, true)
		nextCs.ClearAllRefs()
	} else {
		nextNod, err = loadNode(ctx, nextCs)
		if err != nil {
			return nil, nil, err
		}
	}

	// Publish the replacement root through the transaction.
	t.setRootCursor(nextCs, nextNod)
	return removedNodCursor, removedNod, nil
}

// setFromRoot calls setFromNode from the root of the tree.
// if valueCursor == nil, sets an empty block ref.
func (t *Tx) setFromRoot(ctx context.Context, key []byte, valueCursor *block.Cursor, isBlob bool) error {
	// Prepare the current root for insertion into an empty or populated tree.
	bcs := t.bcs
	nextRoot := t.root
	if nextRoot == nil {
		nextRoot = &Node{}
	}

	// Insert the value and retain the resulting root.
	var changed bool
	nextRoot, bcs, changed, err := t.setFromNode(ctx, bcs, nextRoot, key, valueCursor, isBlob)
	if !changed || err != nil {
		return err
	}

	// Publish the changed root through the transaction.
	t.setRootCursor(bcs, nextRoot)
	return nil
}

// getFromRoot calls getFromNode at the root of the tree.
// returns the *block.Cursor located at the node.
func (t *Tx) getFromRoot(ctx context.Context, key []byte) (*block.Cursor, *Node, error) {
	return t.getFromNode(ctx, t.bcs, t.root, key)
}

// getFromNode finds a key in a sub-tree.
// returns the *block.Cursor located at the node.
func (t *Tx) getFromNode(
	ctx context.Context,
	bcs *block.Cursor,
	n *Node,
	key []byte,
) (*block.Cursor, *Node, error) {
	if n.IsLeaf() {
		if bytes.Equal(n.GetKey(), key) {
			return bcs, n, nil
		}
		// not found
		return nil, nil, nil
	}
	ln, lcs, _, err := t.followKeyFromNode(ctx, bcs, n, key)
	if err != nil {
		return nil, nil, err
	}
	return t.getFromNode(ctx, lcs, ln, key)
}

type batchLookup struct {
	key   []byte
	index int
}

func (t *Tx) getBatchFromNode(
	ctx context.Context,
	bcs *block.Cursor,
	n *Node,
	lookups []batchLookup,
	values [][]byte,
	found []bool,
) error {
	// Finish batch traversal at a missing subtree.
	if n == nil {
		return nil
	}

	// Resolve the batch keys that match this leaf.
	if n.IsLeaf() {
		for _, lookup := range lookups {
			if !bytes.Equal(n.GetKey(), lookup.key) {
				continue
			}
			value, err := t.nodeToValue(ctx, bcs, n)
			if err != nil {
				return err
			}
			values[lookup.index] = value
			found[lookup.index] = true
		}
		return nil
	}

	// Partition batch lookups by the Node separator.
	var leftLookups []batchLookup
	var rightLookups []batchLookup
	for _, lookup := range lookups {
		if bytes.Compare(lookup.key, n.GetKey()) < 0 {
			leftLookups = append(leftLookups, lookup)
		} else {
			rightLookups = append(rightLookups, lookup)
		}
	}

	// Resolve the batch keys routed to the left subtree.
	if len(leftLookups) != 0 {
		leftNode, leftCursor, err := n.FollowLeft(ctx, bcs)
		if err != nil {
			return err
		}
		if err := t.getBatchFromNode(ctx, leftCursor, leftNode, leftLookups, values, found); err != nil {
			return err
		}
	}

	// Resolve the batch keys routed to the right subtree.
	if len(rightLookups) != 0 {
		rightNode, rightCursor, err := n.FollowRight(ctx, bcs)
		if err != nil {
			return err
		}
		if err := t.getBatchFromNode(ctx, rightCursor, rightNode, rightLookups, values, found); err != nil {
			return err
		}
	}
	return nil
}

// setRootCursor updates the root cursor and object.
func (t *Tx) setRootCursor(bcs *block.Cursor, root *Node) {
	t.root = root
	t.bcs = bcs
	if t.tx != nil {
		_ = t.tx.SetRoot(bcs)
	}
	if t.rootChangedCb != nil {
		t.rootChangedCb(bcs)
	}
}

// followKeyFromNode follows left or right by comparing node keys.
func (t *Tx) followKeyFromNode(
	ctx context.Context,
	bcs *block.Cursor,
	n *Node,
	key []byte,
) (ln *Node, lcs *block.Cursor, left bool, err error) {
	left = bytes.Compare(key, n.GetKey()) < 0
	if left {
		ln, lcs, err = n.FollowLeft(ctx, bcs)
	} else {
		ln, lcs, err = n.FollowRight(ctx, bcs)
	}
	return
}

// hasFromNode checks if a key exists in a sub-tree.
func (t *Tx) hasFromNode(ctx context.Context, bcs *block.Cursor, n *Node, key []byte) (bool, error) {
	// Recognize a matching Node key before descending.
	if bytes.Equal(n.GetKey(), key) {
		return true, nil
	}

	// Finish an unsuccessful search at a leaf.
	if n.IsLeaf() {
		return false, nil
	}

	// Follow the child subtree selected by the requested key.
	ln, lcs, _, err := t.followKeyFromNode(ctx, bcs, n, key)
	if err != nil {
		return false, err
	}
	return t.hasFromNode(ctx, lcs, ln, key)
}

// setNodeValue sets the value of a node, handling both blob and non-blob cases.
func (t *Tx) setNodeValue(ctx context.Context, cs *block.Cursor, nod *Node, valCursor *block.Cursor, isBlob bool) error {
	if isBlob {
		// Ensure the value cursor has a valid blob block.
		if valCursor == nil {
			valCursor = cs.Detach(false)
			valCursor.ClearAllRefs()
			valCursor.SetBlock(nil, true)
		}
		if blk, _ := valCursor.GetBlock(); blk == nil {
			if valCursor.GetRef().GetEmpty() {
				// ref was empty and blk was empty. set a empty blob
				valCursor.SetBlock(blob.NewBlobBlock(), false)
			} else {
				// unmarshal the blob at the ref before setting as a sub-block.
				if _, err := blob.UnmarshalBlob(ctx, valCursor); err != nil {
					return err
				}
			}
		}

		// Set the blob as a sub-block.
		if err := valCursor.SetAsSubBlock(8, cs); err != nil {
			return err
		}
	} else {
		// For non-blob values, set the value reference.
		nod.ValueRef = valCursor.GetRef()
		cs.SetRef(7, valCursor)
	}
	return nil
}

// createLeafNode creates a new leaf node with the given key and value.
func (t *Tx) createLeafNode(ctx context.Context, cs *block.Cursor, key []byte, valCursor *block.Cursor, isBlob bool) (*Node, *block.Cursor, error) {
	// Install a keyed leaf on a cursor with cleared references.
	nod := &Node{
		Key:  key,
		Size: 1,
	}
	cs.ClearAllRefs()
	cs.SetBlock(nod, true)

	// Attach the requested value to the new leaf.
	if err := t.setNodeValue(ctx, cs, nod, valCursor, isBlob); err != nil {
		return nil, nil, err
	}

	return nod, cs, nil
}

// setFromNode sets a key recursively from a node.
func (t *Tx) setFromNode(
	ctx context.Context,
	bcs *block.Cursor,
	nod *Node,
	key []byte,
	valCursor *block.Cursor,
	isBlob bool,
) (*Node, *block.Cursor, bool, error) {
	// Replace a matching leaf or split it around the inserted key.
	if nod.IsLeaf() {
		keyCmp := bytes.Compare(key, nod.GetKey())
		if keyCmp == 0 || nod.GetSize() == 0 {
			// Re-initialize the node with the new key and value.
			nod.Key = key
			nod.Size = 1
			nod.Height = 0
			nod.LeftChildRef = nil
			nod.RightChildRef = nil
			nod.ValueBlob = nil
			nod.ValueRef = nil

			// Replace the leaf block and discard its previous cursor references.
			bcs.SetBlock(nod, true)
			bcs.ClearAllRefs()

			// Attach the replacement value to the reinitialized leaf.
			if err := t.setNodeValue(ctx, bcs, nod, valCursor, isBlob); err != nil {
				return nod, bcs, true, err
			}

			return nod, bcs, true, nil
		}

		// Create a new root node for the sub-graph.
		nrootNod := &Node{Height: 1, Size: 2}
		nroot := bcs.Detach(false)
		nroot.ClearAllRefs()
		nroot.SetBlock(nrootNod, true)

		// ncs points to the new block containing key and value.
		var ncs *block.Cursor
		if keyCmp < 0 {
			// key is less than the node's key; set nroot's right child to the current node.
			nrootNod.Key = nod.Key
			nroot.SetRef(6, bcs)
			ncs = nroot.FollowRef(5, nil)
		} else {
			// key is greater than the node's key; set nroot's left child to the current node.
			nrootNod.Key = key
			nroot.SetRef(5, bcs)
			ncs = nroot.FollowRef(6, nil)
		}

		// Create a new leaf node with the key and value.
		if _, _, err := t.createLeafNode(ctx, ncs, key, valCursor, isBlob); err != nil {
			return nrootNod, nroot, true, err
		}

		return nrootNod, nroot, true, nil
	}

	// Recursive case for non-leaf nodes.
	nextNod, nextBc, left, err := t.followKeyFromNode(ctx, bcs, nod, key)
	if err != nil {
		return nil, nil, false, err
	}

	// Insert into the selected child subtree.
	_, setCs, changed, err := t.setFromNode(ctx, nextBc, nextNod, key, valCursor, isBlob)
	if err != nil {
		return nil, nil, changed, err
	}
	if !changed {
		return nod, bcs, false, nil
	}

	// Update the child reference with the new subtree.
	if left {
		bcs.SetRef(5, setCs)
	} else {
		bcs.SetRef(6, setCs)
	}

	// Recompute the parent height and size from the updated children.
	leftNod, leftCs, rightNod, rightCs, err := t.loadNodeChildren(ctx, nod, bcs)
	if err != nil {
		return nil, nil, changed, err
	}
	nod.Height = max(leftNod.GetHeight(), rightNod.GetHeight()) + 1
	nod.Size = leftNod.GetSize() + rightNod.GetSize()
	bcs.SetBlock(nod, true)

	// Balance the tree from this node.
	nroot, nrootCs, err := t.balanceFromLoadedChildren(ctx, nod, bcs, leftNod, leftCs, rightNod, rightCs)
	return nroot, nrootCs, true, err
}

// removeFromNode recursively removes the key balancing the tree
// returns:
// - a cursor to the node that replaces the original
// - the new leftmost key for the tree
// - the key that replaces the orig. node after remove
// - the orphaned cursor to the old node
// - the old node
// - error
func (t *Tx) removeFromNode(
	ctx context.Context,
	bcs *block.Cursor,
	nod *Node,
	key []byte,
) (*block.Cursor, []byte, *block.Cursor, *Node, error) {
	// Remove a matching leaf or finish an unsuccessful leaf search.
	if nod.IsLeaf() {
		if bytes.Equal(key, nod.GetKey()) {
			return nil, nil, bcs, nod, nil
		}
		return nil, nil, nil, nil, nil
	}

	// Locate the child subtree containing the deletion key.
	lnod, lcs, left, err := t.followKeyFromNode(ctx, bcs, nod, key)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Remove the key from the selected child subtree.
	ncs, nkey, removedCursor, removedNode, err := t.removeFromNode(ctx, lcs, lnod, key)
	if err != nil || removedNode == nil {
		return nil, nil, nil, nil, err
	}

	// Promote the surviving child when deletion exhausts its sibling.
	if ncs == nil {
		// Promote the surviving child. The separator is the right subtree's
		// minimum, even when that child is an internal node.
		if left {
			_, lcs, err = nod.FollowRight(ctx, bcs)
			return lcs, nod.GetKey(), removedCursor, removedNode, err
		}
		_, lcs, err = nod.FollowLeft(ctx, bcs)
		return lcs, nil, removedCursor, removedNode, err
	}

	// Set the left or right node to new child.
	if left {
		bcs.SetRef(5, ncs)
	} else {
		bcs.SetRef(6, ncs)
		if len(nkey) != 0 {
			nod.Key = nkey
			bcs.SetBlock(nod, true)
		}
		// A changed right minimum updates this separator, not its ancestors.
		nkey = nil
	}

	// Recompute the parent metadata after replacing its child.
	err = t.calcNodeHeightAndSize(ctx, nod, bcs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return bcs, nkey, removedCursor, removedNode, nil
}

// calcNodeHeightAndSize calculates a node's height and size.
func (t *Tx) calcNodeHeightAndSize(ctx context.Context, nod *Node, bcs *block.Cursor) error {
	// Load both children needed to recompute the Node metadata.
	leftNod, _, rightNod, _, err := t.loadNodeChildren(ctx, nod, bcs)
	if err != nil {
		return err
	}

	// Update the Node height and size from its children.
	nod.Height = max(leftNod.GetHeight(), rightNod.GetHeight()) + 1
	nod.Size = leftNod.GetSize() + rightNod.GetSize()
	bcs.SetBlock(nod, true)
	return nil
}

func (t *Tx) loadNodeChildren(
	ctx context.Context,
	nod *Node,
	bcs *block.Cursor,
) (*Node, *block.Cursor, *Node, *block.Cursor, error) {
	// Load the Node's left subtree and cursor.
	leftNod, leftCs, err := nod.FollowLeft(ctx, bcs)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Load the Node's right subtree and cursor.
	rightNod, rightCs, err := nod.FollowRight(ctx, bcs)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return leftNod, leftCs, rightNod, rightCs, nil
}

// calcNodeBalance calculates a node's balance.
func (t *Tx) calcNodeBalance(ctx context.Context, nod *Node, bcs *block.Cursor) (int, error) {
	// Load the left child for the Node balance calculation.
	leftNod, _, err := nod.FollowLeft(ctx, bcs)
	if err != nil {
		return 0, err
	}

	// Load the right child for the Node balance calculation.
	rightNod, _, err := nod.FollowRight(ctx, bcs)
	if err != nil {
		return 0, err
	}
	return int(leftNod.GetHeight()) - int(rightNod.GetHeight()), nil
}

// rotateNodeRight rotates the tree rooted at the node to the right
// the parent link to nod needs to be replaced with a link to the new root
func (t *Tx) rotateNodeRight(ctx context.Context, nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
	// new root node will be nod->left
	leftNod, leftNodCs, err := nod.FollowLeft(ctx, bcs)
	if err != nil {
		return nil, nil, err
	}

	// follow leftNod->right (n4)
	_, leftNodRightCs, err := leftNod.FollowRight(ctx, leftNodCs)
	if err != nil {
		return nil, nil, err
	}

	// leftNod->left remains the same
	// leftNod->right becomes bcs
	// nod->right remains the same
	// nod->left becomes leftNod->right
	// to correctly fix the block graph:
	// 1. set n1->left to n4 (n2->right)
	bcs.SetRef(5, leftNodRightCs)

	// 2. set n2->right to n1
	leftNodCs.SetRef(6, bcs)

	// Recompute the demoted Node metadata after the right rotation.
	err = t.calcNodeHeightAndSize(ctx, nod, bcs)
	if err != nil {
		return nil, nil, err
	}

	// Recompute the promoted Node metadata after the right rotation.
	err = t.calcNodeHeightAndSize(ctx, leftNod, leftNodCs)
	if err != nil {
		return nil, nil, err
	}

	return leftNod, leftNodCs, nil
}

// rotateNodeLeft rotates the tree rooted at the node to the left
// the parent link to nod needs to be replaced with a link to the new root
func (t *Tx) rotateNodeLeft(ctx context.Context, nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
	// new root node will be nod->right
	rightNod, rightNodCs, err := nod.FollowRight(ctx, bcs)
	if err != nil {
		return nil, nil, err
	}

	// follow rightNod->left (n3)
	// n3 may be a leaf
	_, rightNodLeftCs, err := rightNod.FollowLeft(ctx, rightNodCs)
	if err != nil {
		return nil, nil, err
	}

	// rightnod->right remains the same
	// nod->right becomes rightnod->left
	bcs.SetRef(6, rightNodLeftCs)

	// rightnod->left becomes nod
	rightNodCs.SetRef(5, bcs)

	// Recompute the demoted Node metadata after the left rotation.
	err = t.calcNodeHeightAndSize(ctx, nod, bcs)
	if err != nil {
		return nil, nil, err
	}

	// Recompute the promoted Node metadata after the left rotation.
	err = t.calcNodeHeightAndSize(ctx, rightNod, rightNodCs)
	if err != nil {
		return nil, nil, err
	}

	return rightNod, rightNodCs, nil
}

func (t *Tx) balanceFromLoadedChildren(
	ctx context.Context,
	nod *Node,
	bcs *block.Cursor,
	leftNod *Node,
	leftNodCs *block.Cursor,
	rightNod *Node,
	rightNodCs *block.Cursor,
) (*Node, *block.Cursor, error) {
	balance := int(leftNod.GetHeight()) - int(rightNod.GetHeight())
	if balance > 1 {
		leftNodBalance, err := t.calcNodeBalance(ctx, leftNod, leftNodCs)
		if err != nil {
			return nil, nil, err
		}
		if leftNodBalance < 0 {
			// left right case
			// set nod->left to rotateLeft(nod->left)
			_, lrCs, err := t.rotateNodeLeft(ctx, leftNod, leftNodCs)
			if err != nil {
				return nil, nil, err
			}
			bcs.SetRef(5, lrCs)
			err = t.calcNodeHeightAndSize(ctx, nod, bcs)
			if err != nil {
				return nil, nil, err
			}
		} // else left case

		return t.rotateNodeRight(ctx, nod, bcs)
	}
	if balance < -1 {
		rightNodBalance, err := t.calcNodeBalance(ctx, rightNod, rightNodCs)
		if err != nil {
			return nil, nil, err
		}
		if rightNodBalance > 0 {
			// set nod->right to rotateRight(nod->right)
			_, rrCs, err := t.rotateNodeRight(ctx, rightNod, rightNodCs)
			if err != nil {
				return nil, nil, err
			}
			bcs.SetRef(6, rrCs)
			err = t.calcNodeHeightAndSize(ctx, nod, bcs)
			if err != nil {
				return nil, nil, err
			}
		} // else right case

		return t.rotateNodeLeft(ctx, nod, bcs)
	}

	return nod, bcs, nil
}

// nodeToValue converts a node into a []byte value, depending on isBlob flag.
func (t *Tx) nodeToValue(ctx context.Context, bcs *block.Cursor, n *Node) ([]byte, error) {
	valueCursor, isBlob := n.FollowValue(bcs)
	if isBlob {
		return blob.FetchToBytes(ctx, valueCursor)
	}
	dat, _, err := valueCursor.Fetch(ctx)
	return dat, err
}

// scanLeaves calls cb with each keyed leaf under nod in [start, end), in
// ascending order. An empty start or end leaves that side unbounded.
func (t *Tx) scanLeaves(
	ctx context.Context,
	bcs *block.Cursor, nod *Node,
	start, end []byte,
	cb func(*block.Cursor, *Node) error,
) error {
	// Compare the node key with both bounds.
	nkey := nod.GetKey()
	afterStart := len(start) == 0 || bytes.Compare(start, nkey) < 0
	startOrAfter := len(start) == 0 || bytes.Compare(start, nkey) <= 0
	beforeEnd := len(end) == 0 || bytes.Compare(nkey, end) < 0

	// Report a leaf inside the range.
	if nod.IsLeaf() {
		if len(nkey) == 0 || !startOrAfter || !beforeEnd {
			return nil
		}
		return cb(bcs, nod)
	}

	// Visit the left subtree when it can hold keys at or after start.
	if afterStart {
		left, leftCs, err := nod.FollowLeft(ctx, bcs)
		if err != nil {
			return err
		}
		if err := t.scanLeaves(ctx, leftCs, left, start, end, cb); err != nil {
			return err
		}
	}

	// Visit the right subtree when it can hold keys before end.
	if !beforeEnd {
		return nil
	}
	right, rightCs, err := nod.FollowRight(ctx, bcs)
	if err != nil {
		return err
	}
	return t.scanLeaves(ctx, rightCs, right, start, end, cb)
}

// _ is a type assertion
var (
	_ kvtx.Tx      = (*Tx)(nil)
	_ kvtx.BlockTx = (*Tx)(nil)
)
