package btree

import (
	"sync"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/object"
	"github.com/aperturerobotics/hydra/kvtx"
)

// degree is the degree of the tree.
const degree = 3

// BTree is an implementation of a object-store backed BTree.
// The key is a string, and the value is a object reference.
type BTree struct {
	// rmtx guards the tree
	rmtx       sync.RWMutex
	rootCursor *object.Cursor
	freeList   sync.Pool
}

// NewBTree creates a btree handle with an optionalroot object cursor pointing to
// the tree. The cursor ref can be empty to indicate a new tree is being created.
func NewBTree(
	rootCursor *object.Cursor,
) *BTree {
	if rootCursor == nil {
		return nil
	}

	return &BTree{
		rootCursor: rootCursor,
		freeList:   sync.Pool{New: func() interface{} { return &Node{} }},
	}
}

// NewTransaction returns a new transaction against the store.
// Indicate write if the transaction will not be read-only.
// Always call Discard() after you are done with the transaction.
func (b *BTree) NewTransaction(write bool) (kvtx.Tx, error) {
	return b.NewBTreeTransaction(write)
}

// NewBTreeTransaction returns a transaction with additional btree functionality.
func (b *BTree) NewBTreeTransaction(write bool) (*Tx, error) {
	if write {
		b.rmtx.Lock()
	} else {
		b.rmtx.RLock()
	}

	rn, baseNod, tx, rnCursor, baseNodCursor, err := b.fetchRoot()
	atx := &Tx{
		b:             b,
		write:         write,
		rn:            rn,
		baseNod:       baseNod,
		tx:            tx,
		rnCursor:      rnCursor,
		baseNodCursor: baseNodCursor,
	}
	if err != nil {
		atx.Discard()
		return nil, err
	}
	return atx, nil
}

// GetRootNodeRef returns the reference to the root node.
func (b *BTree) GetRootNodeRef() *object.ObjectRef {
	b.rmtx.RLock()
	defer b.rmtx.RUnlock()

	return b.rootCursor.GetRef()
}

// fetchRoot fetches the root block.
func (b *BTree) fetchRoot() (
	r *Root,
	rn *Node,
	btx *block.Transaction,
	bcs, rnCursor *block.Cursor,
	err error,
) {
	btx, bcs = b.rootCursor.BuildTransaction(nil)
	if !b.rootCursor.GetRef().GetRootRef().GetEmpty() {
		bi, biErr := bcs.Unmarshal(func() block.Block {
			return &Root{}
		})
		if biErr != nil {
			return nil, nil, nil, nil, nil, biErr
		}
		r, _ = bi.(*Root)
	} else {
		r = &Root{}
		bcs.SetBlock(r)
	}
	rnRef := r.GetRootNodeRef()
	rnCursor, err = bcs.FollowRef(1, rnRef)
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if r.GetLength() != 0 {
		rni, err := rnCursor.Unmarshal(b.newNodeBlock)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		rnn, ok := rni.(*Node)
		if ok {
			rn = rnn
		}
	}
	return
}

// newNode builds a new node from the free list.
func (b *BTree) newNode() *Node {
	fget := b.freeList.Get()
	var n *Node
	if fget != nil {
		n = fget.(*Node)
		n.N = 0
		n.Leaf = false
		if n.ChildrenRefs != nil {
			n.ChildrenRefs = n.ChildrenRefs[:0]
		}
		if n.Items != nil {
			n.Items = n.Items[:0]
		}
		n.Reset()
	} else {
		n = &Node{}
	}
	return n
}

// newNodeBlock builds a new node block from the free list.
func (b *BTree) newNodeBlock() block.Block {
	return b.newNode()
}

// _ is a type assertion
var _ kvtx.Store = ((*BTree)(nil))
