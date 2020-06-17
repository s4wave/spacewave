package auth_challenge

import "errors"

var (
	// ErrNotFound indicates the entity was not found.
	ErrNotFound = errors.New("entity not found")
)
