package bldr_plugin

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// pluginManifestRefSet holds a set of PluginManifestRef.
type pluginManifestRefSet struct {
	v *[]*PluginManifestRef
}

// NewPluginManifestRefSet builds a new pluginManifestRefSet container.
//
// bcs should be located at the pluginManifestRefSet sub-block.
func NewPluginManifestRefSet(v *[]*PluginManifestRef, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&pluginManifestRefSet{v: v}, bcs)
}

// NewPluginManifestSetSubBlockCtor returns the sub-block constructor.
func NewPluginManifestSetSubBlockCtor(r *[]*PluginManifestRef) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		return NewPluginManifestRefSet(r, nil)
	}
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *pluginManifestRefSet) Get(i int) block.SubBlock {
	refs := *r.v
	if len(refs) > i {
		return refs[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *pluginManifestRefSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *pluginManifestRefSet) Set(i int, ref block.SubBlock) {
	refs := *r.v
	if i < 0 || i >= len(refs) {
		return
	}
	refs[i], _ = ref.(*PluginManifestRef)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *pluginManifestRefSet) Truncate(nlen int) {
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
var _ sbset.SubBlockContainer = ((*pluginManifestRefSet)(nil))
