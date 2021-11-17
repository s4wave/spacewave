package git_world

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// headRefStoreSet holds a set of HeadRefStore objects.
type headRefStoreSet struct {
	v *[]*HeadRefStore
}

// NewHeadRefStoreSet builds a new head ref store set container.
//
// bcs should be located at the world change set sub-block.
func NewHeadRefStoreSet(v *[]*HeadRefStore, bcs *block.Cursor) *sbset.NamedSubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewNamedSubBlockSet(&headRefStoreSet{v: v}, bcs)
}

// Get returns the value at the tree.
//
// Return nil if out of bounds, etc.
func (r *headRefStoreSet) Get(i int) block.NamedSubBlock {
	c := *r.v
	if len(c) > i {
		return c[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *headRefStoreSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the tree.
func (r *headRefStoreSet) Set(i int, ref block.NamedSubBlock) {
	c := *r.v
	if i < 0 || i >= len(c) {
		return
	}
	c[i], _ = ref.(*HeadRefStore)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *headRefStoreSet) Truncate(nlen int) {
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
var _ sbset.NamedSubBlockContainer = ((*headRefStoreSet)(nil))
