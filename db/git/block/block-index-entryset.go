package git_block

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// indexEntrySet holds a set of IndexEntry objects.
type indexEntrySet struct {
	v *[]*IndexEntry
}

// NewIndexEntrySet builds a new index entry set container.
//
// bcs should be located at the world change set sub-block.
func NewIndexEntrySet(v *[]*IndexEntry, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&indexEntrySet{v: v}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *indexEntrySet) Get(i int) block.SubBlock {
	c := *r.v
	if len(c) > i {
		return c[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *indexEntrySet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *indexEntrySet) Set(i int, ref block.SubBlock) {
	c := *r.v
	if i < 0 || i >= len(c) {
		return
	}
	c[i], _ = ref.(*IndexEntry)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *indexEntrySet) Truncate(nlen int) {
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
var _ sbset.SubBlockContainer = ((*indexEntrySet)(nil))
