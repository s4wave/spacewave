package kvtx_block_iavl

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// Iterator implements a iterator over the iavl tree.
// TODO implement efficient iterator
type Iterator struct {
	// underlying kvtx iterator, caching all keys in memory
	*kvtx_iterator.Iterator
	// keyValueBcs is the block cursor for the value for the current key.
	keyValueBcs *block.Cursor
}

// kvtxIteratorOps implements Get() with GetWithCursor()
type kvtxIteratorOps struct {
	*Tx
	it *Iterator
}

// Get returns values for a key.
func (o *kvtxIteratorOps) Get(ctx context.Context, key []byte) (data []byte, found bool, err error) {
	nodCs, nod, err := o.Tx.getFromRoot(ctx, key)
	if err != nil || nod == nil || nodCs == nil {
		return nil, false, err
	}
	if nod.ValueIsBlob() {
		o.it.keyValueBcs = nodCs.FollowSubBlock(8)
	} else {
		o.it.keyValueBcs = nodCs.FollowRef(7, nod.GetValueRef())
	}
	data, err = o.Tx.nodeToValue(ctx, nodCs, nod)
	if err != nil {
		return nil, true, err
	}
	return data, true, nil
}

// _ is a type assertion
var _ kvtx.TxOps = ((*kvtxIteratorOps)(nil))

// NewIterator constructs a new iavl iterator.
func NewIterator(ctx context.Context, t *Tx, prefix []byte, sort, reverse bool) *Iterator {
	ops := &kvtxIteratorOps{Tx: t}
	n := &Iterator{
		Iterator: kvtx_iterator.NewIterator(ctx, ops, prefix, sort, reverse),
	}
	ops.it = n
	return n
}

// GetValueCursor returns the cursor located at the current value sub-block.
// Returns nil if !valid.
func (i *Iterator) ValueCursor() *block.Cursor {
	// ensure value was fetched
	// this calls Get() internally which sets keyBcs
	_, _ = i.Iterator.Value()
	return i.keyValueBcs
}

// _ is a type assertion
var (
	_ kvtx.Iterator      = ((*Iterator)(nil))
	_ kvtx.BlockIterator = ((*Iterator)(nil))
)
