package world_block_tx

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// txSet holds a world block transaction set.
type txSet struct {
	txs *[]*Tx
}

// newTxSetContainer builds a new transaction set container.
//
// bcs should be located at the slice sub-block
func newTxSetContainer(t *[]*Tx, bcs *block.Cursor) *sbset.SubBlockSet {
	if t == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&txSet{txs: t}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *txSet) Get(i int) block.SubBlock {
	v := *r.txs
	if len(v) == 0 || i >= len(v) {
		return nil
	}
	return v[i]
}

// Len returns the number of elements.
func (r *txSet) Len() int {
	v := *r.txs
	return len(v)
}

// Set sets the value at the index.
func (r *txSet) Set(i int, ref block.SubBlock) {
	v := *r.txs
	if i < 0 || i >= len(v) {
		return
	}
	tx, ok := ref.(*Tx)
	if ok {
		v[i] = tx
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *txSet) Truncate(nlen int) {
	olen := r.Len()
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		*r.txs = nil
	} else {
		v := *r.txs
		for i := nlen; i < olen; i++ {
			v[i] = nil
		}
		*r.txs = v[:nlen]
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*txSet)(nil))
