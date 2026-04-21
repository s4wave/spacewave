package forge_execution

import "errors"

// ErrUnknownState is returned if the state was unknown.
var ErrUnknownState = errors.New("unknown execution state")
