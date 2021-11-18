package forge_target

import "errors"

var (
	// ErrTargetWorldUnset is returned if no target world was set.
	ErrTargetWorldUnset = errors.New("no target world configured")
	// ErrUnknownInputType is returned if the inpuut type is unknown.
	ErrUnknownInputType = errors.New("unknown input type")
	// ErrUnexpectedInputValueType is returned if the input value type was unexpected.
	ErrUnexpectedInputValueType = errors.New("unexpected input value type")
)
