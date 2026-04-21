package block_transform

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// stepConfigSet holds a set of step configurations.
type stepConfigSet struct {
	v *[]*StepConfig
}

// NewStepConfigSet builds a new step config set container.
//
// bcs should be located at the step config set sub-block.
func NewStepConfigSet(v *[]*StepConfig, bcs *block.Cursor) *sbset.SubBlockSet {
	if v == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&stepConfigSet{v: v}, bcs)
}

// NewStepConfigSetSubBlockCtor returns the sub-block constructor.
func NewStepConfigSetSubBlockCtor(r *[]*StepConfig) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		return NewStepConfigSet(r, nil)
	}
}

// IsNil returns if the object is nil.
func (r *stepConfigSet) IsNil() bool {
	return r == nil
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *stepConfigSet) Get(i int) block.SubBlock {
	vals := *r.v
	if len(vals) > i {
		return vals[i]
	}
	return nil
}

// Len returns the number of elements.
func (r *stepConfigSet) Len() int {
	return len(*r.v)
}

// Set sets the value at the index.
func (r *stepConfigSet) Set(i int, ref block.SubBlock) {
	vals := *r.v
	if i < 0 || i >= len(vals) {
		return
	}
	vals[i], _ = ref.(*StepConfig)
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *stepConfigSet) Truncate(nlen int) {
	vals := *r.v
	olen := len(vals)
	if nlen < 0 || nlen >= olen {
		return
	}
	for i := nlen; i < olen; i++ {
		vals[i] = nil
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*stepConfigSet)(nil))
