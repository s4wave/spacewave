package enabled

import "errors"

// IsEnabled returns whether the option is enabled, given the default value.
func (x Enabled) IsEnabled(defaultValue bool) bool {
	switch x {
	case Enabled_DEFAULT:
		return defaultValue
	case Enabled_ENABLE:
		return true
	case Enabled_DISABLE:
		return false
	default:
		return defaultValue
	}
}

// Validate returns an error if the Enabled value is invalid.
func (x Enabled) Validate() error {
	switch x {
	case Enabled_DEFAULT, Enabled_ENABLE, Enabled_DISABLE:
		return nil
	default:
		return errors.New("invalid enabled value: " + x.String())
	}
}

// Merge merges y into x overriding x if y is not set to DEFAULT.
func (x Enabled) Merge(y Enabled) Enabled {
	if y == Enabled_DEFAULT {
		return x
	}
	return y
}
