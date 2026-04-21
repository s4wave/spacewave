package forge_value

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// valueSlice holds a set of Value objects.
type valueSlice struct {
	v *[]*Value
}

// NewValueSubBlockSet builds a new value set container.
//
// bcs should be located at the sub-block
func NewValueSubBlockSet(v *[]*Value, bcs *block.Cursor) *sbset.NamedSubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewNamedSubBlockSet(&valueSlice{v: v}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *valueSlice) Get(i int) sbset.NamedSubBlock {
	v := *r.v
	if len(v) == 0 || i < 0 || i >= len(v) {
		return nil
	}
	return v[i]
}

// Len returns the number of elements.
func (r *valueSlice) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *valueSlice) Set(i int, ref sbset.NamedSubBlock) {
	v := *r.v
	if i < 0 || i >= len(v) {
		return
	}
	iv, ok := ref.(*Value)
	if ok {
		v[i] = iv
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *valueSlice) Truncate(nlen int) {
	rv := *r.v
	olen := len(rv)
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		*r.v = nil
	} else {
		for i := nlen; i < olen; i++ {
			rv[i] = nil
		}
		*r.v = rv[:nlen]
	}
}

// _ is a type assertion
var _ sbset.NamedSubBlockContainer = ((*valueSlice)(nil))
