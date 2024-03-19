package web_runtime

import "errors"

// ErrEmptyWebRuntimeID is returned if the web runtime ID was empty.
var ErrEmptyWebRuntimeID = errors.New("web runtime id cannot be empty")
