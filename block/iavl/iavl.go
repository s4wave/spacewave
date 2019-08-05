// Package iavl implements a iavl tree.
//
// NOTE: This code package is similar to Tendermint IAVL:
// https://github.com/tendermint/iavl
// ...and may be subject to its Apache 2 license.
package iavl

import (
	"bytes"
	"errors"
	"strings"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/object"
)

var ErrEmptyValue = errors.New("cannot set empty value")

// AVLTree is a AVL+ tree. Changes are performed by creating a new
// tree with some internal pointers to parts of the previous tree.
type AVLTree struct {
	rootCursor *object.Cursor
	root       *Node
}

// NewAVLTree builds a new immutable container with a root cursor.
func NewAVLTree(rootCursor *object.Cursor) *AVLTree {
	return &AVLTree{rootCursor: rootCursor}
}

// LoadAVLTree loads a immutable container by following a root cursor.
func LoadAVLTree(
	rootCursor *object.Cursor,
) (*AVLTree, error) {
	t := NewAVLTree(rootCursor)
	_, bcs := rootCursor.BuildTransaction(nil)
	rn, err := loadNode(bcs)
	if err != nil {
		return nil, err
	}
	t.root = rn
	return t, nil
}

// GetRootRef returns the current root reference.
func (t *AVLTree) GetRootRef() *object.ObjectRef {
	return t.rootCursor.GetRef()
}

// Size returns the number of keys in the tree.
func (t *AVLTree) Size() uint64 {
	return t.root.GetSize()
}

// Height returns the height of the tree.
func (t *AVLTree) Height() uint32 {
	return t.root.GetHeight()
}

// Has returns whether or not a key exists.
func (t *AVLTree) Has(key string) (bool, error) {
	if t.root.GetSize() == 0 {
		return false, nil
	}
	_, bcs := t.rootCursor.BuildTransaction(nil)
	return t.hasFromNode(bcs, t.root, key)
}

// hasFromNode checks if a key exists in a sub-tree.
func (t *AVLTree) hasFromNode(
	cursor *block.Cursor,
	n *Node,
	key string,
) (bool, error) {
	if n.GetKey() == key {
		return true, nil
	}
	if n.IsLeaf() {
		return false, nil
	}
	var ln *Node
	var lcs *block.Cursor
	var err error
	if key < n.GetKey() {
		ln, lcs, err = n.FollowLeft(cursor)
	} else {
		ln, lcs, err = n.FollowRight(cursor)
	}
	if err != nil {
		return false, err
	}
	return t.hasFromNode(lcs, ln, key)
}

// Get returns the index and value of the specified key if it exists, or nil
// and the next index, if it doesn't.
func (t *AVLTree) Get(key string) ([]byte, bool, error) {
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}
	_, lcs := t.rootCursor.BuildTransaction(nil)
	return t.getFromNode(lcs, t.root, key)
}

// getFromNode finds a key in a sub-tree.
func (t *AVLTree) getFromNode(
	cursor *block.Cursor,
	n *Node,
	key string,
) ([]byte, bool, error) {
	if n.IsLeaf() {
		if n.GetKey() == key {
			return n.GetValue(), true, nil
		}
		return nil, false, nil
	}
	var ln *Node
	var lcs *block.Cursor
	var err error
	if key < n.GetKey() {
		ln, lcs, err = n.FollowLeft(cursor)
	} else {
		ln, lcs, err = n.FollowRight(cursor)
	}
	if err != nil {
		return nil, false, err
	}
	return t.getFromNode(lcs, ln, key)
}

// Set sets a key to a value.
func (t *AVLTree) Set(key string, val []byte) (changed bool, err error) {
	if len(val) == 0 {
		return false, ErrEmptyValue
	}

	btx, bcs := t.rootCursor.BuildTransaction(nil)
	defer t.finalizeTransaction(&err, btx)
	if t.root == nil {
		t.root = &Node{}
	}

	bcs.SetBlock(t.root)
	if t.root.Size == 0 {
		t.root.Key = key
		t.root.Value = val
		t.root.Size = 1
		return true, nil
	}
	nextNod, nextCs, changed, err := t.setFromNode(bcs, t.root, key, val)
	if !changed || err != nil {
		return changed, err
	}
	btx.SetRoot(nextCs)
	t.root = nextNod // TODO: check if this needs to be reverted on error
	return true, nil
}

// Remove removes a key from the tree.
func (t *AVLTree) Remove(key string) (_ []byte, _ bool, err error) {
	btx, bcs := t.rootCursor.BuildTransaction(nil)
	defer t.finalizeTransaction(&err, btx)
	if t.root == nil {
		t.root = &Node{}
	}
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}

	nextCs, _, val, removed, err := t.removeFromNode(bcs, t.root, key)
	if err != nil {
		return nil, false, err
	}
	nextNod, err := loadNode(nextCs)
	if err != nil {
		return nil, false, err
	}
	t.root = nextNod // TODO: check if this needs to be reverted on error
	btx.SetRoot(nextCs)
	return val, removed, nil
}

// setFromNode sets a key recursively from a node.
func (t *AVLTree) setFromNode(
	bcs *block.Cursor,
	nod *Node,
	key string,
	val []byte,
) (*Node, *block.Cursor, bool, error) {
	if nod.IsLeaf() {
		switch strings.Compare(key, nod.Key) {
		case -1:
			// create new right node equiv to old nod
			bcs.ClearRef(6)
			n1RightCs, err := bcs.FollowRef(6, nil)
			if err != nil {
				return nil, nil, false, err
			}
			n1RightCs.SetBlock(&Node{
				Key:   nod.GetKey(),
				Value: nod.GetValue(),
				Size:  1,
			})
			nod.Height = 1
			nod.Size = 2
			nod.Value = nil
			nod.RightChildRef = nil
			nod.LeftChildRef = nil
			bcs.SetBlock(nod)

			bcs.ClearRef(5)
			ncs, err := bcs.FollowRef(5, nil)
			if err != nil {
				return nil, nil, false, err
			}
			ncs.SetBlock(&Node{
				Key:   key,
				Value: val,
				Size:  1,
			})
			return nod, bcs, true, nil
		case 1:
			// create new left node equiv to old nod
			bcs.ClearRef(5)
			n1LeftCs, err := bcs.FollowRef(5, nil)
			if err != nil {
				return nil, nil, false, err
			}
			n1LeftCs.SetBlock(&Node{
				Key:   nod.GetKey(),
				Value: nod.GetValue(),
				Size:  1,
			})
			nod.Key = key
			nod.Height = 1
			nod.Size = 2
			nod.Value = nil
			nod.RightChildRef = nil
			nod.LeftChildRef = nil
			bcs.SetBlock(nod)

			bcs.ClearRef(6)
			ncs, err := bcs.FollowRef(6, nil)
			if err != nil {
				return nil, nil, false, err
			}
			ncs.SetBlock(&Node{
				Key:   key,
				Value: val,
				Size:  1,
			})
			return nod, bcs, true, nil
		default:
			if bytes.Compare(nod.GetValue(), val) == 0 {
				return nod, bcs, false, nil
			}
			nod.Key = key
			nod.Value = val
			nod.Size = 1
			nod.Height = 0
			nod.LeftChildRef = nil
			nod.RightChildRef = nil
			bcs.ClearRef(5)
			bcs.ClearRef(6)
			bcs.SetBlock(nod)
			return nod, bcs, true, nil
		}
	}

	var err error
	var nextBc *block.Cursor
	var nextNod *Node
	left := key < nod.GetKey()
	if left {
		nextNod, nextBc, err = nod.FollowLeft(bcs)
	} else {
		nextNod, nextBc, err = nod.FollowRight(bcs)
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
func (t *AVLTree) removeFromNode(
	bcs *block.Cursor,
	nod *Node,
	key string,
) (*block.Cursor, string, []byte, bool, error) {
	if nod.IsLeaf() {
		if key == nod.GetKey() {
			// todo: add nod to free pool
			return nil, "", nod.GetValue(), true, nil
		}
		return nil, "", nil, false, nil
	}

	left := key < nod.GetKey()
	var lnod *Node
	var lcs *block.Cursor
	var err error
	if left {
		lnod, lcs, err = nod.FollowLeft(bcs)
	} else {
		lnod, lcs, err = nod.FollowRight(bcs)
	}
	if err != nil {
		return nil, "", nil, false, err
	}
	ncs, nkey, value, removed, err := t.removeFromNode(lcs, lnod, key)
	if err != nil || !removed {
		return nil, "", nil, removed, err
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
		return lcs, nod.GetKey(), value, removed, err
	}

	// Set the left or right node to new child.
	if left {
		bcs.SetRef(5, ncs)
	} else {
		bcs.SetRef(6, ncs)
		if len(nkey) != 0 {
			nod.Key = nkey
			bcs.SetBlock(nod)
		}
	}
	err = t.calcNodeHeightAndSize(nod, bcs)
	if err != nil {
		return nil, "", nil, false, err
	}
	_, balCs, err := t.balanceFromNode(nod, bcs)
	if err != nil {
		return nil, "", nil, false, err
	}
	return balCs, "", value, removed, nil
}

// calcNodeHeightAndSize calcluates a node's height and size.
func (t *AVLTree) calcNodeHeightAndSize(nod *Node, bcs *block.Cursor) error {
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
	bcs.SetBlock(nod)
	return nil
}

// calcNodeBalance calcluates a node's balance
func (t *AVLTree) calcNodeBalance(nod *Node, bcs *block.Cursor) (int, error) {
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
func (t *AVLTree) rotateNodeRight(nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
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
func (t *AVLTree) rotateNodeLeft(nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
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
func (t *AVLTree) balanceFromNode(nod *Node, bcs *block.Cursor) (*Node, *block.Cursor, error) {
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

// finalizeTransaction finalizes a write transaction.
func (t *AVLTree) finalizeTransaction(rerr *error, tx *block.Transaction) {
	if *rerr == nil {
		res, _, err := tx.Write()
		if err != nil || len(res) == 0 {
			*rerr = err
			return
		}
		rb := res[len(res)-1]
		br := rb.GetPutBlock().GetBlockCommon().GetBlockRef()
		t.rootCursor.SetRootRef(br)
	}
}

// ScanPrefix iterates over keys with a prefix.
// Ascending.
func (t *AVLTree) ScanPrefix(prefix string, cb func(key string, val []byte) error) error {
	if t.root.GetSize() == 0 {
		return nil
	}
	_, bcs := t.rootCursor.BuildTransaction(nil)
	return t.traverseFromNode(
		t.root,
		bcs,
		prefix,
		prefix+"\U0010FFFF",
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

// traverseFromNode traverses the tree starting at the node
func (t *AVLTree) traverseFromNode(
	nod *Node, bcs *block.Cursor,
	start, end string,
	ascending, inclusive bool,
	depth uint8,
	cb func(*Node, uint8) error,
) error {
	hasStart := start != ""
	hasEnd := end != ""
	nkey := nod.GetKey()
	afterStart := !hasStart || start < nkey
	startOrAfter := !hasStart || start <= nkey
	beforeEnd := !hasEnd || nkey < end
	if inclusive {
		beforeEnd = !hasEnd || nod.GetKey() <= end
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
