package forge_target

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// NewValueSet constructs a new value set.
func NewValueSet() *ValueSet {
	return &ValueSet{}
}

// NewValueSetBlock constructs a new value set block.
func NewValueSetBlock() block.Block {
	return NewValueSet()
}

// NewValueSetSubBlockCtor returns the sub-block constructor.
func NewValueSetSubBlockCtor(r **ValueSet) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if create && v == nil {
			v = &ValueSet{}
			*r = v
		}
		return v
	}
}

// IsNil checks if the object is nil.
func (v *ValueSet) IsNil() bool {
	return v == nil
}

// Validate performs cursory checks of the ValueSet.
func (v *ValueSet) Validate() error {
	for idx, inp := range v.GetInputs() {
		if err := inp.Validate(false); err != nil {
			return errors.Wrapf(err, "inputs[%d]", idx)
		}
	}
	for idx, out := range v.GetOutputs() {
		if err := out.Validate(false); err != nil {
			return errors.Wrapf(err, "outputs[%d]", idx)
		}
	}

	if !block.IsNamedSubBlocksSorted(v.GetInputs()) {
		return errors.New("inputs: must be sorted by name")
	}
	if !block.IsNamedSubBlocksSorted(v.GetOutputs()) {
		return errors.New("outputs: must be sorted by name")
	}

	return nil
}

// SortValues sorts the inputs and outputs fields.
func (v *ValueSet) SortValues() {
	var inputSlice forge_value.ValueSlice = v.Inputs
	inputSlice.SortByName()

	var outputSlice forge_value.ValueSlice = v.Outputs
	outputSlice.SortByName()
}

// Clone copies the ValueSet.
func (v *ValueSet) Clone() *ValueSet {
	origInputs, origOutputs := v.GetInputs(), v.GetOutputs()

	inputs := make([]*forge_value.Value, len(origInputs))
	for i, inp := range origInputs {
		inputs[i] = inp.Clone()
	}

	outputs := make([]*forge_value.Value, len(origOutputs))
	for i, outp := range origOutputs {
		outputs[i] = outp.Clone()
	}

	return &ValueSet{
		Inputs:  inputs,
		Outputs: outputs,
	}
}

// LookupInput looks up the input with the given name in the list.
// returns nil, -1 if not found.
func (v *ValueSet) LookupInput(name string) (*forge_value.Value, int) {
	for i, inp := range v.GetInputs() {
		if inp.GetName() == name {
			return inp, i
		}
	}
	return nil, -1
}

// LookupOutput looks up the output with the given name in the list.
// returns nil, -1 if not found.
func (v *ValueSet) LookupOutput(name string) (*forge_value.Value, int) {
	for i, oup := range v.GetOutputs() {
		if oup.GetName() == name {
			return oup, i
		}
	}
	return nil, -1
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (v *ValueSet) MarshalBlock() ([]byte, error) {
	return v.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (v *ValueSet) UnmarshalBlock(data []byte) error {
	return v.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (v *ValueSet) ApplySubBlock(id uint32, next block.SubBlock) error {
	// ignore: sub-block set always points to field in v
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (v *ValueSet) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	if inp := v.GetInputs(); inp != nil {
		m[1] = forge_value.NewValueSubBlockSet(&v.Inputs, nil)
	}
	if out := v.GetOutputs(); out != nil {
		m[2] = forge_value.NewValueSubBlockSet(&v.Outputs, nil)
	}
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (v *ValueSet) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 1:
		return func(create bool) block.SubBlock {
			return forge_value.NewValueSubBlockSet(&v.Inputs, nil)
		}
	case 2:
		return func(create bool) block.SubBlock {
			return forge_value.NewValueSubBlockSet(&v.Outputs, nil)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*ValueSet)(nil))
	_ block.BlockWithSubBlocks = ((*ValueSet)(nil))
)
