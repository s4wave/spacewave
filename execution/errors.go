package forge_execution

import "errors"

var (
	// ErrUnknownState is returned if the state was unknown.
	ErrUnknownState = errors.New("unknown execution state")
)
