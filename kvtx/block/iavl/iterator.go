package kvtx_block_iavl

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_iterator "github.com/aperturerobotics/hydra/kvtx/iterator"
)

// Iterator implements a iterator over the iavl tree.
// TODO implement efficient iterator
type Iterator struct {
	// underlying kvtx iterator, caching all keys in memory
	*kvtx_iterator.Iterator
	// keyBcs is the block cursor for the current key.
	keyBcs *block.Cursor
}

// kvtxIteratorOps implements Get() with GetWithCursor()
type kvtxIteratorOps struct {
	*Tx
	it *Iterator
}

// Get returns values for a key.
func (o *kvtxIteratorOps) Get(key []byte) (data []byte, found bool, err error) {
	nodCs, nod, err := o.Tx.getFromRoot(key)
	if err != nil || nod == nil || nodCs == nil {
		return nil, false, err
	}
	o.it.keyBcs = nodCs.FollowRef(7, nod.GetValueRef())
	data, err = o.Tx.nodeToValue(o.Tx.ctx, nodCs, nod)
	if err != nil {
		return nil, true, err
	}
	return data, true, nil
}

// _ is a type assertion
var _ kvtx.TxOps = ((*kvtxIteratorOps)(nil))

// NewIterator constructs a new iavl iterator.
func NewIterator(t *Tx, prefix []byte, sort, reverse bool) *Iterator {
	ops := &kvtxIteratorOps{Tx: t}
	n := &Iterator{
		Iterator: kvtx_iterator.NewIterator(ops, prefix, sort, reverse),
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
	return i.keyBcs
}

// _ is a type assertion
var (
	_ kvtx.Iterator      = ((*Iterator)(nil))
	_ kvtx.BlockIterator = ((*Iterator)(nil))
)
