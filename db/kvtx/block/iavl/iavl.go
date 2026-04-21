// Package iavl implements a iavl tree.
//
// NOTE: This code package is similar to Tendermint IAVL:
// https://github.com/tendermint/iavl
// ...and may be subject to its Apache 2 license.
package kvtx_block_iavl

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/kvtx"
)

// AVLTree is a AVL+ tree. Changes are performed by creating a new
// tree with some internal pointers to parts of the previous tree.
type AVLTree struct {
	rmtx       sync.RWMutex
	rootCursor *bucket_lookup.Cursor
	// todo: freeList
}

// NewAVLTree creates a handle with an optional root object cursor pointing to
// the tree. The cursor ref can be empty to indicate a new tree.
func NewAVLTree(rootCursor *bucket_lookup.Cursor) *AVLTree {
	return &AVLTree{rootCursor: rootCursor}
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
func (t *AVLTree) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return t.NewAVLTreeTransaction(ctx, write)
}

// NewAVLTreeTransaction returns a transaction with additional iavl functionality.
func (t *AVLTree) NewAVLTreeTransaction(ctx context.Context, write bool) (*Tx, error) {
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
	atx, err := NewTx(ctx, bcs, btx, write, nil)
	if err != nil {
		rel()
		return nil, err
	}
	atx.t = t
	atx.rel = rel
	return atx, nil
}

// _ is a type assertion
var _ kvtx.Store = ((*AVLTree)(nil))
