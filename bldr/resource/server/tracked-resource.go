package resource_server

import (
	"time"

	"github.com/aperturerobotics/starpc/srpc"
)

type trackedResource struct {
	mux           srpc.Invoker
	value         any
	ownerClientID uint32
	releaseFn     func()

	// ResourceRpc-created resources remain pending until Adopt. The parent and
	// invocation metadata make leaks diagnosable without coupling lifetime to
	// invocation completion.
	parentResourceID uint32
	serviceID        string
	methodID         string
	createdAt        time.Time
	pending          bool
}
