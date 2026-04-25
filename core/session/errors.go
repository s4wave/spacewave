package session

import "errors"

// ErrSessionNotFound is returned if the session was not found.
var ErrSessionNotFound = errors.New("session not found")
