package kvtx_block_iavl

import (
	"bytes"
	"context"
	"errors"
	"sync"

	"github.com/aperturerobotics/bifrost/util/prng"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/restic/chunker"
	// kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// Tx is a iavl transaction
type Tx struct {
	ctx   context.Context
	write bool

	bcs  *block.Cursor
	root *Node

	t          *AVLTree
	tx         *block.Transaction
	rel        func()
	commitOnce sync.Once

	// rootChangedCb is called if the root cursor changed.
	// may be nil
	rootChangedCb func(*block.Cursor)
}

// NewTx constructs a new IAVL transaction decoupled from the tree, commit and
// discard will be no-op. Note: the root of the tree will change after many set
// operations, it will be necessary to update any references as well.
//
// btx may be nil, if set, will call Write() on it when Commit() is called.
func NewTx(
	ctx context.Context,
	bcs *block.Cursor, btx *block.Transaction,
	write bool,
	rootChangedCb func(*block.Cursor),
) (*Tx, error) {
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
		ctx:           ctx,
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
	t.commitOnce.Do(func() {
		if t.write && t.tx != nil {
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
	t.commitOnce.Do(func() {
		if t.rel != nil {
			t.rel()
		}
	})
}

// Size returns the number of keys in the tree.
func (t *Tx) Size() (uint64, error) {
	return t.root.GetSize(), nil
}

// Height returns the height of the tree.
func (t *Tx) Height() uint32 {
	return t.root.GetHeight()
}

// Exists returns whether or not a key exists.
func (t *Tx) Exists(key []byte) (bool, error) {
	if len(key) == 0 {
		return false, kvtx.ErrEmptyKey
	}
	if t.root.GetSize() == 0 {
		return false, nil
	}
	return t.hasFromNode(t.bcs, t.root, key)
}

// Get returns the value of the specified key if it exists.
func (t *Tx) Get(key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}

	if t.root.GetSize() == 0 {
		return nil, false, nil
	}

	bcs, node, err := t.getFromRoot(key)
	if err != nil || node == nil || bcs == nil {
		return nil, false, err
	}
	val, err := t.nodeToValue(bcs, node)
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}

// GetCursorAtKey returns the cursor at the specified key, if it exists.
// If the key was updated with Set(), points to a Blob.
//
// Returns nil, nil if not found.
func (t *Tx) GetCursorAtKey(key []byte) (*block.Cursor, error) {
	if len(key) == 0 {
		return nil, kvtx.ErrEmptyKey
	}
	if t.root.GetSize() == 0 {
		return nil, nil
	}
	bcs, nod, err := t.getFromRoot(key)
	if err != nil || bcs == nil || nod == nil {
		return nil, err
	}
	return bcs.FollowRef(7, nod.GetValueRef()), nil
}

// Set sets a key to a value.
// Uses a Blob internally to chunk large data.
func (t *Tx) Set(key []byte, val []byte) (err error) {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}

	// write the blob
	var valueCursor *block.Cursor
	if len(val) != 0 {
		valueCursor = t.bcs.Detach(false)
		valueCursor.ClearAllRefs()
		rdr := bytes.NewReader(val)
		// if you need to override the defaults, use SetCursorAtKey instead.
		// derive the chunking pol psuedorandomly from the key
		rnd := prng.BuildSeededRand(key, []byte{115, 52})
		pol, err := chunker.DerivePolynomial(rnd)
		if err != nil {
			return err
		}
		// stores into valueCursor
		_, err = blob.BuildBlob(
			t.ctx,
			int64(len(val)), rdr,
			valueCursor,
			&blob.BuildBlobOpts{
				ChunkingPol: uint64(pol),
			},
		)
		if err != nil {
			return err
		}
	}
	return t.setFromRoot(key, valueCursor, true)
}

// SetCursorAtKey sets the key to a reference to the object at bcs.
// if bcs == nil, the key is set with a empty block ref.
// bcs must not point to a sub-block.
func (t *Tx) SetCursorAtKey(key []byte, bcs *block.Cursor, isBlob bool) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	if bcs != nil && bcs.IsSubBlock() {
		return errors.New("cannot set sub-block as ref")
	}
	return t.setFromRoot(key, bcs, isBlob)
}

// Delete removes a key from the tree
func (t *Tx) Delete(key []byte) error {
	if len(key) == 0 {
		return kvtx.ErrEmptyKey
	}
	_, _, err := t.GetAndDelete(key)
	return err
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
		t.bcs,
		t.root,
		prefix,
		end,
		true, true, 0,
		func(bcs *block.Cursor, n *Node, _ uint8) error {
			if n.GetHeight() == 0 &&
				len(n.GetKey()) != 0 {
				nodValue, err := t.nodeToValue(bcs, n)
				if err != nil {
					return err
				}
				return cb(n.GetKey(), nodValue)
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

// BlockIterate returns the block iterator.
func (t *Tx) BlockIterate(prefix []byte, sort, reverse bool) kvtx.BlockIterator {
	return NewIterator(t, prefix, sort, reverse)
}

// DeleteCursorAtKey deletes the key and returns the cursor to the value.
// returns nil, nil if not found.
func (t *Tx) DeleteCursorAtKey(key []byte) (*block.Cursor, error) {
	if len(key) == 0 {
		return nil, kvtx.ErrEmptyKey
	}
	if t.root == nil {
		t.root = &Node{}
	}
	if t.root.GetSize() == 0 {
		return nil, nil
	}

	removedNodCursor, removedNod, err := t.removeFromRoot(key)
	if err != nil {
		return nil, err
	}
	return removedNodCursor.FollowRef(7, removedNod.GetValueRef()), nil
}

// GetAndDelete removes a key from the tree returning a value.
func (t *Tx) GetAndDelete(key []byte) (_ []byte, _ bool, err error) {
	if len(key) == 0 {
		return nil, false, kvtx.ErrEmptyKey
	}
	if t.root == nil {
		t.root = &Node{}
	}
	if t.root.GetSize() == 0 {
		return nil, false, nil
	}

	rcs, err := t.DeleteCursorAtKey(key)
	if err != nil || rcs == nil {
		return nil, false, err
	}

	removedBcs, removedNod, err := t.removeFromRoot(key)
	val, err := t.nodeToValue(removedBcs, removedNod)
	return val, true, err
}

// removeFromRoot removes the key from the root and returns the cursor to the removed node.
func (t *Tx) removeFromRoot(key []byte) (*block.Cursor, *Node, error) {
	nextCs, _, removedNodCursor, removedNod, err := t.removeFromNode(t.bcs, t.root, key)
	if err != nil || removedNod == nil {
		return nil, nil, err
	}
	var nextNod *Node
	if nextCs == nil {
		nextCs = t.bcs
		nextNod = &Node{}
		nextCs.SetBlock(nextNod, true)
		nextCs.ClearRef(5)
		nextCs.ClearRef(6)
	} else {
		nextNod, err = loadNode(nextCs)
		if err != nil {
			return nil, nil, err
		}
	}
	t.setRootCursor(nextCs, nextNod)
	return removedNodCursor, removedNod, nil
}

// setFromRoot calls setFromNode from the root of the tree.
// if valueCursor == nil, sets an empty block ref.
func (t *Tx) setFromRoot(key []byte, valueCursor *block.Cursor, isBlob bool) error {
	bcs := t.bcs
	nextRoot := t.root
	if nextRoot == nil {
		nextRoot = &Node{}
	}
	var changed bool
	nextRoot, bcs, changed, err := t.setFromNode(bcs, nextRoot, key, valueCursor, isBlob)
	if !changed || err != nil {
		return err
	}
	t.setRootCursor(bcs, nextRoot)
	return nil
}

// getFromRoot calls getFromNode at the root of the tree.
// returns the *block.Cursor located at the node.
func (t *Tx) getFromRoot(key []byte) (*block.Cursor, *Node, error) {
	return t.getFromNode(t.bcs, t.root, key)
}

// getFromNode finds a key in a sub-tree.
// returns the *block.Cursor located at the node.
func (t *Tx) getFromNode(
	bcs *block.Cursor,
	n *Node,
	key []byte,
) (*block.Cursor, *Node, error) {
	if n.IsLeaf() {
		if bytes.Compare(n.GetKey(), key) == 0 {
			return bcs, n, nil
		}
		// not found
		return nil, nil, nil
	}
	ln, lcs, _, err := t.followKeyFromNode(bcs, n, key)
	if err != nil {
		return nil, nil, err
	}
	return t.getFromNode(lcs, ln, key)
}

// setRootCursor updates the root cursor and object.
func (t *Tx) setRootCursor(bcs *block.Cursor, root *Node) {
	t.root = root
	t.bcs = bcs
	if t.tx != nil {
		t.tx.SetRoot(bcs)
	}
	if t.rootChangedCb != nil {
		t.rootChangedCb(bcs)
	}
}

// followKeyFromNode follows left or right by comparing node keys.
func (t *Tx) followKeyFromNode(
	bcs *block.Cursor,
	n *Node,
	key []byte,
) (ln *Node, lcs *block.Cursor, left bool, err error) {
	left = bytes.Compare(key, n.GetKey()) < 0
	if left {
		ln, lcs, err = n.FollowLeft(bcs)
	} else {
		ln, lcs, err = n.FollowRight(bcs)
	}
	return
}

// hasFromNode checks if a key exists in a sub-tree.
func (t *Tx) hasFromNode(bcs *block.Cursor, n *Node, key []byte) (bool, error) {
	if bytes.Compare(n.GetKey(), key) == 0 {
		return true, nil
	}
	if n.IsLeaf() {
		return false, nil
	}
	ln, lcs, _, err := t.followKeyFromNode(bcs, n, key)
	if err != nil {
		return false, err
	}
	return t.hasFromNode(lcs, ln, key)
}

// setFromNode sets a key recursively from a node.
func (t *Tx) setFromNode(
	bcs *block.Cursor,
	nod *Node,
	key []byte,
	valCursor *block.Cursor,
	isBlob bool,
) (*Node, *block.Cursor, bool, error) {
	// Careful to re-stitch the block graph while maintaining Block objects.
	// To move a block from pos -> pos.right:
	//  - create new block cursor with .Detach(false) (becomes new sub-root)
	//  - create new Node at the new sub-root
	//  - setref from new node -> old node (either left or right)
	//  - set parent child ref -> new child (done when returning)
	if nod.IsLeaf() {
		keyCmp := bytes.Compare(key, nod.GetKey())
		if keyCmp == 0 || nod.GetSize() == 0 {
			// leaf && equal key (or empty) -> override old node
			nod.Key = key
			nod.Size = 1
			nod.Height = 0
			nod.LeftChildRef = nil
			nod.RightChildRef = nil
			nod.ValueRefBlob = isBlob
			nod.ValueRef = valCursor.GetRef()
			bcs.ClearRef(5)
			bcs.ClearRef(6)
			bcs.SetBlock(nod, true)
			bcs.SetRef(7, valCursor)
			return nod, bcs, true, nil
		}

		// create a new root node for the sub-graph
		// if key < node.key, set nroot->rightNode=node, nroot->leftNode=key
		nrootNod := &Node{Height: 1, Size: 2}
		nroot := bcs.Detach(false)
		nroot.ClearAllRefs()
		nroot.SetBlock(nrootNod, true)

		// ncs points to the new block containing key, value
		var ncs *block.Cursor
		if keyCmp < 0 {
			// key is < old key -> set nroot -> right = bcs
			nrootNod.Key = nod.Key
			nroot.SetRef(6, bcs)
			ncs = nroot.FollowRef(5, nil)
		} else {
			// key is > old key -> set nroot -> left = bcs
			nrootNod.Key = key
			nroot.SetRef(5, bcs)
			ncs = nroot.FollowRef(6, nil)
		}

		// set the new node -> the key, val
		ncs.SetBlock(&Node{
			Key:          key,
			Size:         1,
			ValueRefBlob: isBlob,
		}, true)
		ncs.SetRef(7, valCursor)

		return nrootNod, nroot, true, nil
	}

	nextNod, nextBc, left, err := t.followKeyFromNode(bcs, nod, key)
	if err != nil {
		return nil, nil, false, err
	}
	_, setCs, changed, err := t.setFromNode(nextBc, nextNod, key, valCursor, isBlob)
	if err != nil {
		return nil, nil, changed, err
	}
	if !changed {
		return nod, bcs, false, nil
	}
	// set the new sub-tree reference
	if left {
		bcs.SetRef(5, setCs)
	} else {
		bcs.SetRef(6, setCs)
	}

	err = t.calcNodeHeightAndSize(nod, bcs)
	if err != nil {
		return nil, nil, changed, err
	}

	nroot, nrootCs, err := t.balanceFromNode(nod, bcs)
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
	bcs *block.Cursor,
	nod *Node,
	key []byte,
) (*block.Cursor, []byte, *block.Cursor, *Node, error) {
	if nod.IsLeaf() {
		if bytes.Compare(key, nod.GetKey()) == 0 {
			return nil, nil, bcs, nod, nil
		}
		return nil, nil, nil, nil, nil
	}

	lnod, lcs, left, err := t.followKeyFromNode(bcs, nod, key)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	ncs, nkey, removedCursor, removedNode, err := t.removeFromNode(lcs, lnod, key)
	if err != nil || removedNode == nil {
		return nil, nil, nil, nil, err
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
		return lcs, lnod.GetKey(), removedCursor, removedNode, err
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
		return nil, nil, nil, nil, err
	}
	return bcs, nil, removedCursor, removedNode, nil
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

// nodeToValue converts a node into a []byte value, depending on isBlob flag.
func (t *Tx) nodeToValue(bcs *block.Cursor, n *Node) ([]byte, error) {
	valueCursor := bcs.FollowRef(7, n.GetValueRef())
	if n.GetValueRefBlob() {
		return blob.FetchToBytes(t.ctx, valueCursor)
	}
	// empty block returns nil
	dat, _, err := valueCursor.Fetch()
	return dat, err
}

// traverseFromNode traverses the tree starting at the node (recursively)
func (t *Tx) traverseFromNode(
	bcs *block.Cursor, nod *Node,
	start, end []byte,
	ascending, inclusive bool,
	depth uint8,
	cb func(*block.Cursor, *Node, uint8) error,
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
		if err := cb(bcs, nod, depth); err != nil {
			return err
		}
	}
	if leaf {
		return nil
	}

	trav := func(ln *Node, lnCs *block.Cursor) error {
		return t.traverseFromNode(
			lnCs, ln,
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

// _ is a type assertion
var (
	_ kvtx.Tx      = (*Tx)(nil)
	_ kvtx.BlockTx = (*Tx)(nil)
)
