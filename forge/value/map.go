package forge_value

import (
	"github.com/pkg/errors"
)

// ValueMap is a map of named values.
// The key must match the Name field.
type ValueMap map[string]*Value

// Validate checks all values in the map.
func (v ValueMap) Validate(allowNameKeyMismatch bool) error {
	for k, val := range v {
		if err := val.Validate(allowNameKeyMismatch); err != nil {
			return errors.Wrapf(err, "values: %s", k)
		}
		if !allowNameKeyMismatch {
			if valName := val.GetName(); k != valName {
				return errors.Errorf("values: key %s != name %s", k, valName)
			}
		}
	}
	return nil
}
