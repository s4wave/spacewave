package world_mock

import "errors"

var (
	// ErrEmptyNextMsg is returned if the nextmsg field is empty.
	ErrEmptyNextMsg = errors.New("next_msg field cannot be empty")
)
