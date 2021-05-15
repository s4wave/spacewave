package forge_value

import "github.com/pkg/errors"

// ValueSlice is a slice of values.
type ValueSlice []*Value

// Validate checks all values in the slice.
func (v ValueSlice) Validate(checkDupe bool) error {
	m := make(map[string]int)
	for i, val := range v {
		if err := val.Validate(); err != nil {
			return errors.Wrapf(err, "values[%d]", i)
		}
		if checkDupe {
			name := val.GetName()
			if oi, ok := m[name]; ok {
				return errors.Wrapf(
					errors.Wrapf(ErrDuplicateValueName, "%q", name),
					"values[%d] and values[%d]", oi, i,
				)
			}
			m[name] = i
		}
	}
	return nil
}
