package forge_value

import "errors"

var (
	// ErrUnknownValueType indicates the value type was not known.
	ErrUnknownValueType = errors.New("unknown value type")
	// ErrEmptyValueName indicates the value name was empty.
	ErrEmptyValueName = errors.New("value name cannot be empty")
	// ErrDuplicateValueName indicates the value name was duplicated.
	ErrDuplicateValueName = errors.New("duplicate value name")
)
