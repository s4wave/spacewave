package resource_client

// ReferenceCreator adopts resources returned by RPCs on one connection.
// SDK wrappers use it without controlling the connection's transport or load.
type ReferenceCreator interface {
	// CreateResourceReference retains resourceID until the returned ref is released.
	CreateResourceReference(resourceID uint32) ResourceRef
}

// _ verifies that Client supplies resource adoption to SDK wrappers.
var _ ReferenceCreator = (*Client)(nil)
