package space

import "errors"

var (
	// ErrEmptySpaceID is returned if the space id was empty.
	ErrEmptySpaceID = errors.New("space id cannot be empty")
	// ErrSpaceExists is returned if the space already exists.
	ErrSpaceExists = errors.New("space with that id already exists")
)
