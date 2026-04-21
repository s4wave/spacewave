package signaling

import "errors"

// ErrEmptySignalingID is returned if the signaling id cannot be empty.
var ErrEmptySignalingID = errors.New("signaling id cannot be empty")
