package forge_value

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// valueSet holds a set of Value objects.
type valueSet struct {
	v *[]*Value
}

// NewValueSetContainer builds a new value set container.
//
// bcs should be located at the sub-block
func NewValueSetContainer(v *[]*Value, bcs *block.Cursor) *sbset.NamedSubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewNamedSubBlockSet(&valueSet{v: v}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *valueSet) Get(i int) sbset.NamedSubBlock {
	v := *r.v
	if len(v) == 0 || i < 0 || i >= len(v) {
		return nil
	}
	return v[i]
}

// Len returns the number of elements.
func (r *valueSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *valueSet) Set(i int, ref sbset.NamedSubBlock) {
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
func (r *valueSet) Truncate(nlen int) {
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
var _ sbset.NamedSubBlockContainer = ((*valueSet)(nil))
