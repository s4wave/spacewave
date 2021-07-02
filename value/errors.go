package forge_value

import "errors"

var (
	// ErrUnknownValueType indicates the value type was not known.
	ErrUnknownValueType = errors.New("unknown value type")
	// ErrEmptyValueName indicates the value name was empty.
	ErrEmptyValueName = errors.New("value name cannot be empty")
	// ErrDuplicateValueName indicates the value name was duplicated.
	ErrDuplicateValueName = errors.New("duplicate value name")
	// ErrUnsetValue indicates the value was not set.
	ErrUnsetValue = errors.New("value was required but not set")
	// ErrUnexpectedPeerID is returned if the peer id was incorrect.
	ErrUnexpectedPeerID = errors.New("unexpected peer id")
	// ErrUnknownState is returned if the state was unknown/unhandled.
	ErrUnknownState = errors.New("unexpected or unhandled state")
)
