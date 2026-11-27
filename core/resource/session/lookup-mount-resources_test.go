package resource_session_test

import (
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// lookupMountResources retains real mounted resources until test cleanup.
type lookupMountResources struct {
	// ResourceClientContext is unused except for the overridden child registration.
	resource_server.ResourceClientContext
	// release holds the real mount's cleanup after successful registration.
	release func()
}

// AddResource accepts the real SharedObject resource and retains its release.
func (r *lookupMountResources) AddResource(_ srpc.Invoker, release func()) (uint32, error) {
	r.release = release
	return 1, nil
}
