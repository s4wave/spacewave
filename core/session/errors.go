package session

import "errors"

// ErrSessionNotFound is returned if the session was not found.
var ErrSessionNotFound = errors.New("session not found")

// ErrAccountDeleting is returned by a mount of a session of an account with a
// list entry in a state other than ACTIVE: the account's delete has started.
var ErrAccountDeleting = errors.New("the session's account is being deleted")
