package forge_target

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// InputMap is the set of provided input values.
// The key must match the input Name field.
type InputMap map[string]InputValue

// Validate checks all values in the map.
func (m InputMap) Validate() error {
	for k, val := range m {
		if err := val.Validate(); err != nil {
			return errors.Wrap(err, k)
		}
	}
	return nil
}

// BuildValueSet builds a ValueSet from all InlineValue inputs.
func (m InputMap) BuildValueSet() *ValueSet {
	values := make([]*forge_value.Value, 0, len(m))
	for name, inputValue := range m {
		valInline, valInlineOk := inputValue.(InputValueInline)
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
