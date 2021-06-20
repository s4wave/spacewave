package world_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// worldChangeSet holds a set of WorldChange objects.
type worldChangeSet struct {
	v *[]*WorldChange
}

// NewWorldChangeSet builds a new world change set container.
//
// bcs should be located at the world change set sub-block.
func NewWorldChangeSet(v *[]*WorldChange, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&worldChangeSet{v: v}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *worldChangeSet) Get(i int) block.SubBlock {
	c := *r.v
	if len(c) > i {
		return c[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *worldChangeSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *worldChangeSet) Set(i int, ref block.SubBlock) {
	c := *r.v
	if i < 0 || i >= len(c) {
		return
	}
	c[i], _ = ref.(*WorldChange)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *worldChangeSet) Truncate(nlen int) {
	c := *r.v
	olen := len(c)
	if nlen < 0 || nlen >= olen {
		return
	}
	for i := nlen; i < olen; i++ {
		c[i] = nil
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*worldChangeSet)(nil))
