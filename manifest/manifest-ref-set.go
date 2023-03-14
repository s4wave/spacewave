package bldr_manifest

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// manifestRefSet holds a set of ManifestRef.
type manifestRefSet struct {
	v *[]*ManifestRef
}

// NewManifestRefSet builds a new manifestRefSet container.
//
// bcs should be located at the manifestRefSet sub-block.
func NewManifestRefSet(v *[]*ManifestRef, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&manifestRefSet{v: v}, bcs)
}

// NewManifestRefSetSubBlockCtor returns the sub-block constructor.
func NewManifestRefSetSubBlockCtor(r *[]*ManifestRef) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		return NewManifestRefSet(r, nil)
	}
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *manifestRefSet) Get(i int) block.SubBlock {
	refs := *r.v
	if len(refs) > i {
		return refs[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *manifestRefSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *manifestRefSet) Set(i int, ref block.SubBlock) {
	refs := *r.v
	if i < 0 || i >= len(refs) {
		return
	}
	refs[i], _ = ref.(*ManifestRef)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *manifestRefSet) Truncate(nlen int) {
	refs := *r.v
	olen := len(refs)
	if nlen < 0 || nlen >= olen {
		return
	}
	for i := nlen; i < olen; i++ {
		refs[i] = nil
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*manifestRefSet)(nil))
