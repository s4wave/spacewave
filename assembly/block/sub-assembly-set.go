package assembly_block

import (
	block "github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// subAssemblySet holds a list of SubAssembly
type subAssemblySet struct {
	r *[]*SubAssembly
}

// NewSubAssemblySet builds a new SubAssembly slice as a sub-block.
//
// if r is nil, returns nil
// bcs should be located at the sub-block.
func NewSubAssemblySet(r *[]*SubAssembly, bcs *block.Cursor) *sbset.SubBlockSet {
	if r == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&subAssemblySet{r: r}, bcs)
}

// NewSubAssemblySetSubBlockCtor constructs a SubAssemblySet as a SubBlock.
func NewSubAssemblySetSubBlockCtor(r *[]*SubAssembly) block.SubBlockCtor {
	return func(create bool) block.SubBlock {
		if r == nil {
			return nil
		}
		rs := *r
		if len(rs) == 0 && !create {
			return nil
		}
		return NewSubAssemblySet(r, nil)
	}
}

// IsNil checks if the object is nil.
func (r *subAssemblySet) IsNil() bool {
	return r == nil || r.r == nil
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *subAssemblySet) Get(i int) block.SubBlock {
	if r.r == nil {
		return nil
	}
	rs := *r.r
	rslen := len(rs)
	if i >= rslen || i < 0 {
		return nil
	}
	return rs[i]
}

// Len returns the number of elements.
func (r *subAssemblySet) Len() int {
	if r.r == nil {
		return 0
	}
	rs := *r.r
	return len(rs)
}

// Set sets the value at the index.
func (r *subAssemblySet) Set(i int, ref block.SubBlock) {
	if r.r == nil {
		return
	}
	rs := *r.r
	if i < 0 || i >= len(rs) {
		return
	}
	v, ok := ref.(*SubAssembly)
	if ok {
		rs[i] = v
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *subAssemblySet) Truncate(nlen int) {
	olen := r.Len()
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		*r.r = nil
	} else {
		for i := nlen; i < olen; i++ {
			(*r.r)[i] = nil
		}
		*r.r = (*r.r)[:nlen]
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*subAssemblySet)(nil))
