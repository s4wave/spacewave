package iavl

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	// kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// Tx is a iavl transaction
type Tx struct {
	bcs   *block.Cursor
	root  *Node
	write bool

	t          *AVLTree
	tx         *block.Transaction
	rel        func()
	commitOnce sync.Once
}

// NewTx constructs a new IAVL transaction decoupled from the tree, commit and
// discard will be no-op.
func NewTx(bcs *block.Cursor, write bool) (*Tx, error) {
	var rn *Node
	bcsBlk, _ := bcs.GetBlock()
	if bcs.GetRef().GetEmpty() && bcsBlk == nil {
		rn = &Node{}
		bcs.SetBlock(rn, false)
	} else {
		bi, biErr := bcs.Unmarshal(NewNodeBlock)
		if biErr != nil {
			return nil, biErr
		}
		rn, _ = bi.(*Node)
	}
	return &Tx{
		root:  rn,
		bcs:   bcs,
		write: write,
	}, nil
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *Tx) Commit(ctx context.Context) (cerr error) {
	if t.tx == nil || t.t == nil {
		return nil
	}
	t.commitOnce.Do(func() {
		if t.write {
			br, _, err := t.tx.Write(true)
			if err != nil {
				cerr = err
			} else {
				nc := *t.t.rootCursor
				nc.SetRootRef(br)
				t.t.rootCursor = &nc
			}
		}
		if t.rel != nil {
			t.rel()
		}
	})
	return
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (t *Tx) Discard() {
	if t.tx == nil || t.t == nil {
		return
	}
	t.commitOnce.Do(func() {
		if t.rel != nil {
			t.rel()
		}
	})
}

// Size returns the number of keys in the tree.
func (t *Tx) Size() uint64 {
	return t.root.GetSize()
}

// Height returns the height of the tree.
func (t *Tx) Height() uint32 {
	return t.root.GetHeight()
}

// Exists returns whether or not a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	if t.root.GetSize() == 0 {
		return false, nil
	}
	return t.hasFromNode(t.bcs, t.root, key)
}

// hasFromNode checks if a key exists in a sub-tree.
func (t *Tx) hasFromNode(
	cursor *block.Cursor,
	n *Node,
	key []byte,
) (bool, error) {
	if bytes.Compare(n.GetKey(), key) == 0 {
		return true, nil
	}
	if n.IsLeaf() {
		return false, nil
	}
	var ln *Node
	var lcs *block.Cursor
	var err error
	if bytes.Compare(key, n.GetKey()) < 0 {
		ln, lcs, err = n.FollowLeft(cursor)
	} else {
		ln, lcs, err = n.FollowRight(cursor)
	}
	if err != nil {
		return false, err
	}
	return t.hasFromNode(lcs, ln, key)
}

// Get returns the value of the specified key if it exists.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
	val, bcs, err := t.GetWithCursor(key)
	if err != nil {
		return nil, false, err
	}
	if bcs == nil {
		return nil, false, nil
	}
	return val, true, nil
}

// GetWithCursor returns the value of the specified key, if it exists, and a
// block cursor located at the value sub-block. Returns nil, nil, nil if not
// found.
func (t *Tx) GetWithCursor(key []byte) ([]byte, *block.Cursor, error) {
	if t.root.GetSize() == 0 {
		return nil, nil, nil
	}
	return t.getFromNode(t.bcs, t.root, key)
}

// TODO SetWithCursor, set a key with a value sub-block cursor.
// Allows to stitch together two block graphs.

// getFromNode finds a key in a sub-tree.
func (t *Tx) getFromNode(
	cursor *block.Cursor,
	n *Node,
	key []byte,
) ([]byte, *block.Cursor, error) {
	if n.IsLeaf() {
		if bytes.Compare(n.GetKey(), key) == 0 {
			return n.GetValue(), cursor.FollowSubBlock(4), nil
		}
		return nil, nil, nil
	}
	var ln *Node
	var lcs *block.Cursor
	var err error
	if bytes.Compare(key, n.GetKey()) < 0 {
		ln, lcs, err = n.FollowLeft(cursor)
	} else {
		ln, lcs, err = n.FollowRight(cursor)
	}
	if err != nil {
		return nil, nil, err
	}
	return t.getFromNode(lcs, ln, key)
}

// Set sets a key to a value.
func (t *Tx) Set(key []byte, val []byte, ttl time.Duration) (err error) {
	_ = ttl // TODO
	if len(val) == 0 {
		return ErrEmptyValue
	}

	bcs := t.bcs
	if t.root == nil {
		t.root = &Node{}
	}
	bcs.SetBlock(t.root, true)
	if t.root.Size == 0 {
		t.root.Key = key
		t.root.Value = val
		t.root.Size = 1
		return nil
	}
	nextNod, nextCs, changed, err := t.setFromNode(bcs, t.root, key, val)
	if !changed || err != nil {
		return err
	}
	t.tx.SetRoot(nextCs)
	t.root = nextNod
	t.bcs = nextCs
	return nil
}

// Delete removes a key from the tree
func (t *Tx) Delete(key []byte) error {
	_, _, err := t.GetAndDelete(key)
	return err
}

// GetAndDelete removes a key from the tree returning a value.
func (t *Tx) GetAndDelete(key []byte) (_ []byte, _ bool, err error) {
	if t.root == nil {
		t.root = &Node{}
	}
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}

	nextCs, _, val, removed, err := t.removeFromNode(t.bcs, t.root, key)
	if err != nil || !removed {
		return nil, false, err
	}
	if nextCs == nil {
		nextCs = t.bcs
		t.root = &Node{}
		nextCs.SetBlock(t.root, true)
		nextCs.ClearRef(5)
		nextCs.ClearRef(6)
	} else {
		nextNod, err := loadNode(nextCs)
		if err != nil {
			return nil, false, err
		}
		t.root = nextNod
		t.bcs = nextCs
	}
	t.tx.SetRoot(nextCs)
	return val, removed, nil
}

// setFromNode sets a key recursively from a node.
func (t *Tx) setFromNode(
	bcs *block.Cursor,
	nod *Node,
	key []byte,
	val []byte,
) (*Node, *block.Cursor, bool, error) {
	// Careful to re-stitch the block graph while maintaining Block objects.
	// To move a block from pos -> pos.right:
	//  - create new block cursor with .Detach(false) (becomes new sub-root)
	//  - create new Node at the new sub-root
	//  - setref from new node -> old node (either left or right)
	//  - set parent child ref -> new child (done when returning)

	if nod.IsLeaf() {
		keyCmp := bytes.Compare(key, nod.GetKey())
		if keyCmp == 0 {
			// leaf && equal key -> override old node
			nod.Key = key
			nod.Value = val
			nod.Size = 1
			nod.Height = 0
			nod.LeftChildRef = nil
			nod.RightChildRef = nil
			bcs.ClearRef(5)
			bcs.ClearRef(6)
			bcs.SetBlock(nod, true)
			return nod, bcs, true, nil
		}

		// create a new root node for the sub-graph
		nroot := bcs.Detach(false)
		bcs.ClearRef(5) // clear old left_ref
		nod.LeftChildRef = nil
		bcs.ClearRef(6) // clear old right_ref
		nod.RightChildRef = nil
		// clear non-leaf fields on nod
		nod.Height = 0
		nod.Size = 1
		bcs.SetBlock(nod, true)
		// use nod to hold nroot from now on
		nod = &Node{
			Key:    key,
			Height: 1,
			Size:   2,
		}
		nroot.SetBlock(nod, true)
		var ncs *block.Cursor
		if keyCmp < 0 {
			// key is < old key -> set nroot -> right = bcs
			nroot.SetRef(6, bcs)
			ncs = nroot.FollowRef(5, nil)
		} else {
			// key is > old key -> set nroot -> left = bcs
			nroot.SetRef(5, bcs)
			ncs = nroot.FollowRef(6, nil)
		}

		// set the new node -> the key, val
		ncs.SetBlock(&Node{
			Key:   key,
			Value: val,
			Size:  1,
		}, true)
		return nod, nroot, true, nil
	}

	var err error
	var nextBc *block.Cursor
	var nextNod *Node
	left := bytes.Compare(key, nod.GetKey()) < 0
	if left {
		nextNod, nextBc, err = nod.FollowLeft(bcs)
	} else {
		nextNod, nextBc, err = nod.FollowRight(bcs)
	}
	if err != nil {
		return nil, nil, false, err
	}

	_, setCs, changed, err := t.setFromNode(nextBc, nextNod, key, val)
	if err != nil {
		return nil, nil, changed, err
	}
	if !changed {
		return nod, bcs, false, nil
	}
	if left {
		bcs.SetRef(5, setCs)
	} else {
		bcs.SetRef(6, setCs)
	}

	err = t.calcNodeHeightAndSize(nod, bcs)
	if err != nil {
		return nil, nil, false, err
	}

	nroot, nrootCs, err := t.balanceFromNode(nod, bcs)
	return nroot, nrootCs, true, err
}

// removeFromNode recursively removes the key balancing the tree
// returns:
// - a cursor to the node that replaces the original
// - the new leftmost key for the tree
// - the node that replaces the orig. node after remove
// - new leftmost leaf key for tree after successfully removing 'key' if changed.
// - the removed value
// - the orphaned nodes.
func (t *Tx) removeFromNode(
	bcs *block.Cursor,
	nod *Node,
	key []byte,
) (*block.Cursor, []byte, []byte, bool, error) {
	if nod.IsLeaf() {
		if bytes.Compare(key, nod.GetKey()) == 0 {
			// todo: add nod to free pool
			return nil, nil, nod.GetValue(), true, nil
		}
		return nil, nil, nil, false, nil
	}

	left := bytes.Compare(key, nod.GetKey()) < 0
	var lnod *Node
	var lcs *block.Cursor
	var err error
	if left {
		lnod, lcs, err = nod.FollowLeft(bcs)
	} else {
		lnod, lcs, err = nod.FollowRight(bcs)
	}
	if err != nil {
		return nil, nil, nil, false, err
	}
	ncs, nkey, value, removed, err := t.removeFromNode(lcs, lnod, key)
	if err != nil || !removed {
		return nil, nil, nil, removed, err
	}
	if ncs == nil {
		// node was deleted
		// clear ref to child
		// set parent ref (left or right) to other link, deleting this node
		if left {
			lnod, lcs, err = nod.FollowRight(bcs)
		} else {
			lnod, lcs, err = nod.FollowLeft(bcs)
		}
		return lcs, lnod.GetKey(), value, removed, err
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
	}
	err = t.calcNodeHeightAndSize(nod, bcs)
	if err != nil {
		return nil, nil, nil, false, err
	}
	return bcs, nil, value, removed, nil
	/*
		_, balCs, err := t.balanceFromNode(nod, bcs)
		if err != nil {
			return nil, nil, nil, false, err
		}
		return balCs, nil, value, removed, nil
	*/
}

// calcNodeHeightAndSize calcluates a node's height and size.
func (t *Tx) calcNodeHeightAndSize(nod *Node, bcs *block.Cursor) error {
	leftNod, _, err := nod.FollowLeft(bcs)
	if err != nil {
		return err
	}
	rightNod, _, err := nod.FollowRight(bcs)
	if err != nil {
		return err
	}
	nod.Height = maxUint32(leftNod.GetHeight(), rightNod.GetHeight()) + 1
	nod.Size = leftNod.GetSize() + rightNod.GetSize()
	bcs.SetBlock(nod, true)
	return nil
}

// calcNodeBalance calcluates a node's balance
func (t *Tx) calcNodeBalance(nod *Node, bcs *block.Cursor) (int, error) {
	leftNod, _, err := nod.FollowLeft(bcs)
	if err != nil {
		return 0, err
	}
	rightNod, _, err := nod.FollowRight(bcs)
	if err != nil {
		return 0, err
	}
	return int(leftNod.GetHeight()) - int(rightNod.GetHeight()), nil
}

// rotateNodeRight rotates the tree rooted at the node to the right
// the parent link to nod needs to be replaced with a link to the new root
func (t *Tx) rotateNodeRight(nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
	// new root node will be nod->left
	leftNod, leftNodCs, err := nod.FollowLeft(bcs)
	if err != nil {
		return nil, nil, err
	}

	// follow leftNod->right (n4)
	_, leftNodRightCs, err := leftNod.FollowRight(leftNodCs)
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

	err = t.calcNodeHeightAndSize(nod, bcs)
	if err != nil {
		return nil, nil, err
	}
	err = t.calcNodeHeightAndSize(leftNod, leftNodCs)
	if err != nil {
		return nil, nil, err
	}

	return leftNod, leftNodCs, nil
}

// rotateNodeLeft rotates the tree rooted at the node to the left
// the parent link to nod needs to be replaced with a link to the new root
func (t *Tx) rotateNodeLeft(nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
	// new root node will be nod->right
	rightNod, rightNodCs, err := nod.FollowRight(bcs)
	if err != nil {
		return nil, nil, err
	}

	// follow rightNod->left (n3)
	// n3 may be a leaf
	_, rightNodLeftCs, err := rightNod.FollowLeft(rightNodCs)
	if err != nil {
		return nil, nil, err
	}

	// rightnod->right remains the same
	// nod->right becomes rightnod->left
	bcs.SetRef(6, rightNodLeftCs)
	// rightnod->left becomes nod
	rightNodCs.SetRef(5, bcs)

	err = t.calcNodeHeightAndSize(nod, bcs)
	if err != nil {
		return nil, nil, err
	}
	err = t.calcNodeHeightAndSize(rightNod, rightNodCs)
	if err != nil {
		return nil, nil, err
	}

	return rightNod, rightNodCs, nil
}

// maxUint32 returns the max of two uint32
func maxUint32(i1, i2 uint32) uint32 {
	if i1 > i2 {
		return i1
	}
	return i2
}

// balanceFromNode balances the tree from a node.
func (t *Tx) balanceFromNode(nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
	// compute the tree balance
	balance, err := t.calcNodeBalance(nod, bcs)
	if err != nil {
		return nil, nil, err
	}
	if balance > 1 {
		leftNod, leftNodCs, err := nod.FollowLeft(bcs)
		if err != nil {
			return nil, nil, err
		}
		leftNodBalance, err := t.calcNodeBalance(leftNod, leftNodCs)
		if err != nil {
			return nil, nil, err
		}
		if leftNodBalance < 0 {
			// left right case
			// set nod->left to rotateLeft(nod->left)
			_, lrCs, err := t.rotateNodeLeft(leftNod, leftNodCs)
			if err != nil {
				return nil, nil, err
			}
			bcs.SetRef(5, lrCs)
			err = t.calcNodeHeightAndSize(nod, bcs)
			if err != nil {
				return nil, nil, err
			}
		} // else left left case

		return t.rotateNodeRight(nod, bcs)
	}
	if balance < -1 {
		rightNod, rightNodCs, err := nod.FollowRight(bcs)
		if err != nil {
			return nil, nil, err
		}
		rightNodBalance, err := t.calcNodeBalance(rightNod, rightNodCs)
		if err != nil {
			return nil, nil, err
		}
		if rightNodBalance > 0 {
			// set nod->right to rotateRight(nod->right)
			_, rrCs, err := t.rotateNodeRight(rightNod, rightNodCs)
			if err != nil {
				return nil, nil, err
			}
			bcs.SetRef(6, rrCs)
			err = t.calcNodeHeightAndSize(nod, bcs)
			if err != nil {
				return nil, nil, err
			}
		} // else right right case

		return t.rotateNodeLeft(nod, bcs)
	}

	return nod, bcs, nil
}

// ScanPrefix iterates over keys with a prefix.
// Ascending.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, val []byte) error) error {
	if t.root.GetSize() == 0 {
		return nil
	}
	end := make([]byte, len(prefix)+1)
	copy(end, prefix)
	end[len(end)-1] = 255
	return t.traverseFromNode(
		t.root,
		t.bcs,
		prefix,
		end,
		true, true, 0,
		func(n *Node, _ uint8) error {
			if n.GetHeight() == 0 &&
				len(n.GetValue()) != 0 &&
				len(n.GetKey()) != 0 {
				return cb(n.GetKey(), n.GetValue())
			}
			return nil
		},
	)
}

// ScanPrefixKeys iterates over keys with a prefix.
// Ascending.
func (t *Tx) ScanPrefixKeys(prefix []byte, cb func(key []byte) error) error {
	if t.root.GetSize() == 0 {
		return nil
	}
	return t.ScanPrefix(prefix, func(k, v []byte) error {
		return cb(k)
	})
}

// Iterate returns an iterator with a given key prefix.
//
// Should always return non-nil, with error field filled if necessary.
// Iterates in sorted order, reverse reverses the key iteration.
func (t *Tx) Iterate(prefix []byte, sort, reverse bool) kvtx.Iterator {
	return t.IterateIavl(prefix, sort, reverse)
}

// IterateIavl returns the iavl iterator.
func (t *Tx) IterateIavl(prefix []byte, sort, reverse bool) *Iterator {
	return NewIterator(t, prefix, sort, reverse)
}

// traverseFromNode traverses the tree starting at the node (recursively)
func (t *Tx) traverseFromNode(
	nod *Node, bcs *block.Cursor,
	start, end []byte,
	ascending, inclusive bool,
	depth uint8,
	cb func(*Node, uint8) error,
) error {
	hasStart := len(start) != 0
	hasEnd := len(end) != 0
	nkey := nod.GetKey()
	afterStart := !hasStart || bytes.Compare(start, nkey) < 0
	startOrAfter := !hasStart || bytes.Compare(start, nkey) <= 0
	beforeEnd := !hasEnd || bytes.Compare(nkey, end) < 0
	if inclusive {
		beforeEnd = !hasEnd || bytes.Compare(nod.GetKey(), end) <= 0
	}

	leaf := nod.IsLeaf()
	if !leaf || (startOrAfter && beforeEnd) {
		if err := cb(nod, depth); err != nil {
			return err
		}
	}
	if leaf {
		return nil
	}

	trav := func(ln *Node, lnCs *block.Cursor) error {
		return t.traverseFromNode(
			ln, lnCs,
			start, end,
			ascending, inclusive,
			depth+1, cb,
		)
	}
	chk := func(follow func(*block.Cursor) (*Node, *block.Cursor, error)) error {
		ln, lncs, err := follow(bcs)
		if err != nil {
			return err
		}
		return trav(ln, lncs)
	}

	if ascending {
		// check lower nodes, then higher
		if afterStart {
			if err := chk(nod.FollowLeft); err != nil {
				return err
			}
		}
		if beforeEnd {
			if err := chk(nod.FollowRight); err != nil {
				return err
			}
		}
	} else {
		// check the higher nodes first
		if beforeEnd {
			if err := chk(nod.FollowRight); err != nil {
				return err
			}
		}
		if afterStart {
			if err := chk(nod.FollowLeft); err != nil {
				return err
			}
		}
	}

	return nil
}

/*

// GetByIndex gets the key and value at the specified index.
func (t *ImmutableTree) GetByIndex(index int64) (key []byte, value []byte) {
	if t.root == nil {
		return nil, nil
	}
	return t.root.getByIndex(t, index)
}

// Iterate iterates over all keys of the tree, in order.
func (t *ImmutableTree) Iterate(fn func(key []byte, value []byte) bool) (stopped bool) {
	if t.root == nil {
		return false
	}
	return t.root.ltraverse(t, true, func(node *Node) bool {
		if node.height == 0 {
			return fn(node.key, node.value)
		}
		return false
	})
}

// IterateRange makes a callback for all nodes with key between start and end non-inclusive.
// If either are nil, then it is open on that side (nil, nil is the same as Iterate)
func (t *ImmutableTree) IterateRange(start, end []byte, ascending bool, fn func(key []byte, value []byte) bool) (stopped bool) {
	if t.root == nil {
		return false
	}
	return t.root.traverseInRange(t, start, end, ascending, false, 0, func(node *Node, _ uint8) bool {
		if node.height == 0 {
			return fn(node.key, node.value)
		}
		return false
	})
}

// IterateRangeInclusive makes a callback for all nodes with key between start and end inclusive.
// If either are nil, then it is open on that side (nil, nil is the same as Iterate)
func (t *ImmutableTree) IterateRangeInclusive(start, end []byte, ascending bool, fn func(key, value []byte, version int64) bool) (stopped bool) {
	if t.root == nil {
		return false
	}
	return t.root.traverseInRange(t, start, end, ascending, true, 0, func(node *Node, _ uint8) bool {
		if node.height == 0 {
			return fn(node.key, node.value, node.version)
		}
		return false
	})
}

// Clone creates a clone of the tree.
// Used internally by MutableTree.
func (t *ImmutableTree) clone() *ImmutableTree {
	return &ImmutableTree{
		root:    t.root,
		ndb:     t.ndb,
		version: t.version,
	}
}

// nodeSize is like Size, but includes inner nodes too.
func (t *ImmutableTree) nodeSize() int {
	size := 0
	t.root.traverse(t, true, func(n *Node) bool {
		size++
		return false
	})
	return size
}
*/
