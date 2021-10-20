package forge_target

import "errors"

var (
	// ErrTargetWorldUnset is returned if no target world was set.
	ErrTargetWorldUnset = errors.New("no target world configured")
)
