package forge_pass

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// execStateSlice holds a set of ExecState objects.
type execStateSlice struct {
	v *[]*ExecState
}

// NewExecStateSubBlockSet builds a new value set container.
//
// bcs should be located at the sub-block
func NewExecStateSubBlockSet(v *[]*ExecState, bcs *block.Cursor) *sbset.NamedSubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewNamedSubBlockSet(&execStateSlice{v: v}, bcs)
}

// NewExecStateSubBlockSetCtor returns the sub-block constructor.
func NewExecStateSubBlockSetCtor(v *[]*ExecState) block.SubBlockCtor {
	if v == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		return NewExecStateSubBlockSet(v, nil)
	}
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *execStateSlice) Get(i int) sbset.NamedSubBlock {
	v := *r.v
	if len(v) == 0 || i < 0 || i >= len(v) {
		return nil
	}
	return v[i]
}

// Len returns the number of elements.
func (r *execStateSlice) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *execStateSlice) Set(i int, ref sbset.NamedSubBlock) {
	v := *r.v
	if i < 0 || i >= len(v) {
		return
	}
	iv, ok := ref.(*ExecState)
	if ok {
		v[i] = iv
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *execStateSlice) Truncate(nlen int) {
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
var _ sbset.NamedSubBlockContainer = ((*execStateSlice)(nil))
