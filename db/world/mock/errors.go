package world_mock

import "errors"

// ErrEmptyNextMsg is returned if the nextmsg field is empty.
var ErrEmptyNextMsg = errors.New("next_msg field cannot be empty")
