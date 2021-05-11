package forge_value

import "errors"

var (
	// ErrUnknownValueType indicates the value type was not known.
	ErrUnknownValueType = errors.New("unknown value type")
)
