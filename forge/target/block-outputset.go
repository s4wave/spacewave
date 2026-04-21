package forge_target

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// outputSet holds the output set.
type outputSet struct {
	v *[]*Output
}

// newOutputSetContainer builds a new set container.
//
// bcs should be located at the sub-block
func newOutputSetContainer(v *[]*Output, bcs *block.Cursor) *sbset.NamedSubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewNamedSubBlockSet(&outputSet{v: v}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *outputSet) Get(i int) sbset.NamedSubBlock {
	v := *r.v
	if len(v) == 0 || i < 0 || i >= len(v) {
		return nil
	}
	return v[i]
}

// Len returns the number of elements.
func (r *outputSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *outputSet) Set(i int, ref sbset.NamedSubBlock) {
	v := *r.v
	if i < 0 || i >= len(v) {
		return
	}
	iv, ok := ref.(*Output)
	if ok {
		v[i] = iv
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *outputSet) Truncate(nlen int) {
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
var _ sbset.NamedSubBlockContainer = ((*outputSet)(nil))
