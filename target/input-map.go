package forge_target

import (
	"github.com/pkg/errors"
)

// InputMap is the set of provided input values.
// The key must match the input Name field.
type InputMap map[string]InputValue

// Validate checks all values in the map.
func (v InputMap) Validate() error {
	for k, val := range v {
		if err := val.Validate(); err != nil {
			return errors.Wrap(err, k)
		}
	}
	return nil
}
