package forge_target

import "errors"

var (
	// ErrTargetWorldUnset is returned if no target world was set.
	ErrTargetWorldUnset = errors.New("no target world configured")
	// ErrUnknownInputType is returned if the input type is unknown.
	ErrUnknownInputType = errors.New("unknown input type")
	// ErrUnknownOutputType is returned if the output type is unknown.
	ErrUnknownOutputType = errors.New("unknown output type")
	// ErrUnexpectedInputValueType is returned if the input value type was unexpected.
	ErrUnexpectedInputValueType = errors.New("unexpected input value type")
)
