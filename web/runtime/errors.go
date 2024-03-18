package web_runtime

import "errors"

var (
	// ErrEmptyWebRuntimeID is returned if the web runtime ID was empty.
	ErrEmptyWebRuntimeID = errors.New("web runtime id cannot be empty")
)
