package resource

import "errors"

var (
	// ErrResourceNotFound is returned when a requested resource does not exist.
	ErrResourceNotFound = errors.New("resource not found")
	// ErrClientReleased is returned when attempting to operate on a released client.
	ErrClientReleased = errors.New("client was released")
	// ErrInvalidResourceID is returned when a resource ID is invalid or out of bounds.
	ErrInvalidResourceID = errors.New("invalid resource id")
	// ErrInvalidClientID is returned when a client ID is invalid or out of bounds.
	ErrInvalidClientID = errors.New("invalid client id")
	// ErrInvalidComponentIDFormat is returned when a component ID is not in the expected format.
	ErrInvalidComponentIDFormat = errors.New("invalid component id format")
	// ErrResourceOrClientReleased is returned when either the resource or client has been released.
	ErrResourceOrClientReleased = errors.New("resource or client was released")
	// ErrNoResourceClientContext is returned if there was no ResourceClientContext.
	ErrNoResourceClientContext = errors.New("no resource client context")
)
