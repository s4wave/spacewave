package forge_target

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// subBlockSet adapts a slice of named sub-blocks to the sbset container
// interface.
type subBlockSet[T sbset.NamedSubBlock] struct {
	v *[]T
}

// newSubBlockSetContainer builds a new set container over the given slice.
//
// bcs should be located at the sub-block.
// Returns nil if v is nil.
func newSubBlockSetContainer[T sbset.NamedSubBlock](v *[]T, bcs *block.Cursor) *sbset.NamedSubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewNamedSubBlockSet(&subBlockSet[T]{v: v}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *subBlockSet[T]) Get(i int) sbset.NamedSubBlock {
	v := *r.v
	if len(v) == 0 || i < 0 || i >= len(v) {
		return nil
	}
	return v[i]
}

// Len returns the number of elements.
func (r *subBlockSet[T]) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *subBlockSet[T]) Set(i int, ref sbset.NamedSubBlock) {
	v := *r.v
	if i < 0 || i >= len(v) {
		return
	}
	if iv, ok := ref.(T); ok {
		v[i] = iv
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *subBlockSet[T]) Truncate(nlen int) {
	rv := *r.v
	olen := len(rv)
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		*r.v = nil
	} else {
		var zero T
		for i := nlen; i < olen; i++ {
			rv[i] = zero
		}
		*r.v = rv[:nlen]
	}
}

// _ is a type assertion
var _ sbset.NamedSubBlockContainer = (*subBlockSet[sbset.NamedSubBlock])(nil)
