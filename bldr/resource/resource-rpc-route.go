package resource

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

// resourceRpcRouteKey carries abandonment to invocations on one route.
type resourceRpcRouteKey struct{}

// resourceRpcRoute retains invocation cleanup until the client closes its route.
type resourceRpcRoute struct {
	// bcast guards abandonment and callback registration.
	bcast broadcast.Broadcast
	// abandoned records the client's explicit abandonment packet.
	abandoned bool
	// callbacks retain invocation children after their handlers return.
	callbacks []func()
}

// OnResourceRpcAbandon registers cleanup for the lifetime of the current route.
// It calls cleanup immediately if the client already abandoned this route.
// Contexts outside HandleResourceRpc have no route and retain their lifetime.
func OnResourceRpcAbandon(ctx context.Context, cleanup func()) {
	// Find the route whose context flows into the invocation.
	route, _ := ctx.Value(resourceRpcRouteKey{}).(*resourceRpcRoute)
	if route == nil {
		return
	}

	// Serialize registration against abandonment without running cleanup locked.
	var abandoned bool
	route.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		abandoned = route.abandoned
		if !abandoned {
			route.callbacks = append(route.callbacks, cleanup)
		}
	})
	if abandoned {
		cleanup()
	}
}

// abandon releases pending invocation children once, including late registrations.
func (r *resourceRpcRoute) abandon() {
	// Transfer cleanup callbacks out of the route under its state lock.
	var callbacks []func()
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.abandoned = true
		callbacks, r.callbacks = r.callbacks, nil
	})

	// Resource callbacks take the generation lock and run outside the route lock.
	for _, cleanup := range callbacks {
		cleanup()
	}
}
