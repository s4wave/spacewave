package git_block

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// resolveUndoEntrySet holds a set of ResolveUndoEntry objects.
type resolveUndoEntrySet struct {
	v *[]*ResolveUndoEntry
}

// NewResolveUndoEntrySet builds a new ResolveUndo entry set container.
//
// bcs should be located at the world change set sub-block.
func NewResolveUndoEntrySet(v *[]*ResolveUndoEntry, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&resolveUndoEntrySet{v: v}, bcs)
}

// Get returns the value at the resolveUndo.
//
// Return nil if out of bounds, etc.
func (r *resolveUndoEntrySet) Get(i int) block.SubBlock {
	c := *r.v
	if len(c) > i {
		return c[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *resolveUndoEntrySet) Len() int {
	return len(*r.v)
}

// Set sets the value at the resolveUndo.
func (r *resolveUndoEntrySet) Set(i int, ref block.SubBlock) {
	c := *r.v
	if i < 0 || i >= len(c) {
		return
	}
	c[i], _ = ref.(*ResolveUndoEntry)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *resolveUndoEntrySet) Truncate(nlen int) {
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
var _ sbset.SubBlockContainer = ((*resolveUndoEntrySet)(nil))
