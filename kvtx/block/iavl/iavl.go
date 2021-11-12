// Package iavl implements a iavl tree.
//
// NOTE: This code package is similar to Tendermint IAVL:
// https://github.com/tendermint/iavl
// ...and may be subject to its Apache 2 license.
package kvtx_block_iavl

import (
	"context"
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/kvtx"
)

// AVLTree is a AVL+ tree. Changes are performed by creating a new
// tree with some internal pointers to parts of the previous tree.
type AVLTree struct {
	ctx        context.Context
	rmtx       sync.RWMutex
	rootCursor *bucket_lookup.Cursor
	// todo: freeList
}

// NewAVLTree creates a handle with an optional root object cursor pointing to
// the tree. The cursor ref can be empty to indicate a new tree.
func NewAVLTree(ctx context.Context, rootCursor *bucket_lookup.Cursor) *AVLTree {
	return &AVLTree{ctx: ctx, rootCursor: rootCursor}
}

// NewAVLTreeSubBlockCtor returns the sub-block constructor.
func NewAVLTreeSubBlockCtor(r **Node) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if create && v == nil {
			v = &Node{}
			*r = v
		}
		return v
	}
}

// GetRootNodeRef returns the reference to the root node.
func (t *AVLTree) GetRootNodeRef() *bucket.ObjectRef {
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

// NewAVLTreeTransaction returns a transaction with additional iavl functionality.
func (t *AVLTree) NewAVLTreeTransaction(write bool) (*Tx, error) {
	if write {
		t.rmtx.Lock()
	} else {
		t.rmtx.RLock()
	}
	rel := func() {
		if write {
			t.rmtx.Unlock()
		} else {
			t.rmtx.RUnlock()
		}
	}

	btx, bcs := t.rootCursor.BuildTransaction(nil)
	atx, err := NewTx(t.ctx, bcs, btx, write, nil)
	if err != nil {
		rel()
		return nil, err
	}
	atx.t = t
	atx.rel = rel
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
	return
}

// _ is a type assertion
var _ kvtx.Store = ((*AVLTree)(nil))
