package resource_client

// ResourceReleaseReason distinguishes terminal handle transitions from diagnostics.
type ResourceReleaseReason uint8

const (
	// ResourceLive identifies a reference that has not retired.
	ResourceLive ResourceReleaseReason = iota
	// ResourceClientReleased identifies explicit local reference disposal.
	ResourceClientReleased
	// ResourceServerReleased identifies explicit server revocation.
	ResourceServerReleased
	// ResourceRouteRefused identifies a typed route rejection, not revocation.
	ResourceRouteRefused
	// ResourceConnectionLost identifies a generation whose transport ended.
	ResourceConnectionLost
)
