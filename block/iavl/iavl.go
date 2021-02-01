// Package iavl implements a iavl tree.
//
// NOTE: This code package is similar to Tendermint IAVL:
// https://github.com/tendermint/iavl
// ...and may be subject to its Apache 2 license.
package iavl

import (
	"errors"
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/object"
	"github.com/aperturerobotics/hydra/kvtx"
)

var ErrEmptyValue = errors.New("cannot set empty value")

// AVLTree is a AVL+ tree. Changes are performed by creating a new
// tree with some internal pointers to parts of the previous tree.
type AVLTree struct {
	rmtx       sync.RWMutex
	rootCursor *object.Cursor
	// todo: freeList
}

// NewAVLTree creates a handle with an optional root object cursor pointing to
// the tree. The cursor ref can be empty to indicate a new tree.
func NewAVLTree(rootCursor *object.Cursor) *AVLTree {
	return &AVLTree{rootCursor: rootCursor}
}

// GetRootNodeRef returns the reference to the root node.
func (t *AVLTree) GetRootNodeRef() *object.ObjectRef {
	t.rmtx.RLock()
	defer t.rmtx.RUnlock()
	return t.rootCursor.GetRef()
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (t *AVLTree) NewTransaction(write bool) (kvtx.Tx, error) {
	return t.NewAVLTreeTransaction(write)
}

// NewBTreeTransaction returns a transaction with additional btree functionality.
func (t *AVLTree) NewAVLTreeTransaction(write bool) (*Tx, error) {
	if write {
		t.rmtx.Lock()
	} else {
		t.rmtx.RLock()
	}

	rn, btx, bcs, err := t.fetchRoot()
	atx := &Tx{
		t:     t,
		write: write,
		root:  rn,
		tx:    btx,
		bcs:   bcs,
	}
	if err != nil {
		atx.Discard()
		return nil, err
	}
	return atx, nil
}

// fetchRoot fetches the root block.
func (t *AVLTree) fetchRoot() (
	rn *Node,
	btx *block.Transaction,
	bcs *block.Cursor,
	err error,
) {
	btx, bcs = t.rootCursor.BuildTransaction(nil)
	if !t.rootCursor.GetRef().GetRootRef().GetEmpty() {
		bi, biErr := bcs.Unmarshal(NewNodeBlock)
		if biErr != nil {
			return nil, nil, nil, biErr
		}
		rn, _ = bi.(*Node)
	} else {
		rn = &Node{}
		bcs.SetBlock(rn)
	}
	return
}

// _ is a type assertion
var _ kvtx.Store = ((*AVLTree)(nil))
