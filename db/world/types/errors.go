package world_types

import "errors"

var (
	// ErrTypeIDEmpty is returned if the given type ID was empty.TypeIDEmpty is returned if the given type ID was empty.
	ErrTypeIDEmpty = errors.New("type ID empty")
	// ErrUnknownObjectType indicates the object type was not known.
	ErrUnknownObjectType = errors.New("unknown object type")
)
