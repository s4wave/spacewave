package forge_target

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// InputMap is the set of provided input values.
// The key must match the input Name field.
type InputMap map[string]inputValue

// ObjectRef resolves the named input to the object its value refers to. It
// refuses an input that is missing, empty or refers to an empty root, since a
// Step reads the object's tree from that root.
func (m InputMap) ObjectRef(name string) (*bucket.ObjectRef, error) {
	// Resolve the input to a non-empty value.
	iv, ok := m[name]
	if !ok {
		return nil, errors.Errorf("input %q not found in input values", name)
	}
	val, err := InputValueToValue(iv)
	if err != nil {
		return nil, errors.Wrapf(err, "input %q", name)
	}
	if val == nil || val.IsEmpty() {
		return nil, errors.Errorf("input %q is empty", name)
	}

	// The value references the root of the object's tree.
	ref, err := val.ToBucketRef()
	if err != nil {
		return nil, errors.Wrapf(err, "input %q", name)
	}
	if ref.GetRootRef().GetEmpty() {
		return nil, errors.Errorf("input %q has empty root ref", name)
	}
	return ref, nil
}

// BuildValueSet builds a ValueSet from all InlineValue inputs.
func (m InputMap) BuildValueSet() *ValueSet {
	values := make([]*forge_value.Value, 0, len(m))
	for name, inputValue := range m {
		valInline, valInlineOk := inputValue.(inputValueInline)
		if !valInlineOk {
			continue
		}

		val := valInline.GetValue().Clone()
		if val == nil {
			continue
		}

		val.Name = name
		values = append(values, val)
	}

	block.SortNamedSubBlocks(values)
	return &ValueSet{Inputs: values}
}
