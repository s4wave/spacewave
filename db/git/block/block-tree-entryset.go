package git_block

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// treeEntrySet holds a set of TreeEntry objects.
type treeEntrySet struct {
	v *[]*TreeEntry
}

// NewTreeEntrySet builds a new tree entry set container.
//
// bcs should be located at the world change set sub-block.
func NewTreeEntrySet(v *[]*TreeEntry, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&treeEntrySet{v: v}, bcs)
}

// Get returns the value at the tree.
//
// Return nil if out of bounds, etc.
func (r *treeEntrySet) Get(i int) block.SubBlock {
	c := *r.v
	if len(c) > i {
		return c[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *treeEntrySet) Len() int {
	return len(*r.v)
}

// Set sets the value at the tree.
func (r *treeEntrySet) Set(i int, ref block.SubBlock) {
	c := *r.v
	if i < 0 || i >= len(c) {
		return
	}
	c[i], _ = ref.(*TreeEntry)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *treeEntrySet) Truncate(nlen int) {
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
var _ sbset.SubBlockContainer = ((*treeEntrySet)(nil))
