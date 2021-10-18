package assembly_block

import (
	block "github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// directiveBridgeSet holds a list of DirectiveBridge
type directiveBridgeSet struct {
	r *[]*DirectiveBridge
}

// NewDirectiveBridgeSet builds a new DirectiveBridge slice as a sub-block.
//
// if r is nil, returns nil
// bcs should be located at the sub-block.
func NewDirectiveBridgeSet(r *[]*DirectiveBridge, bcs *block.Cursor) *sbset.SubBlockSet {
	if r == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&directiveBridgeSet{r: r}, bcs)
}

//  NewDirectiveBridgeSetSubBlockCtor constructs a SubDirectiveBridgeSet as a SubBlock.
func NewDirectiveBridgeSetSubBlockCtor(r *[]*DirectiveBridge) block.SubBlockCtor {
	return func(create bool) block.SubBlock {
		if r == nil {
			return nil
		}
		rs := *r
		if len(rs) == 0 && !create {
			return nil
		}
		return NewDirectiveBridgeSet(r, nil)
	}
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *directiveBridgeSet) Get(i int) block.SubBlock {
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
func (r *directiveBridgeSet) Len() int {
	if r.r == nil {
		return 0
	}
	rs := *r.r
	return len(rs)
}

// Set sets the value at the index.
func (r *directiveBridgeSet) Set(i int, ref block.SubBlock) {
	if r.r == nil {
		return
	}
	rs := *r.r
	if i < 0 || i >= len(rs) {
		return
	}
	v, ok := ref.(*DirectiveBridge)
	if ok {
		rs[i] = v
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *directiveBridgeSet) Truncate(nlen int) {
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
var _ sbset.SubBlockContainer = ((*directiveBridgeSet)(nil))
