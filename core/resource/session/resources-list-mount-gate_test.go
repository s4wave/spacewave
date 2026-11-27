package resource_session_test

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/sobject"
)

// resourcesListMountGate delays one Space's first body mount on the real bus.
type resourcesListMountGate struct {
	// Bus mounts every other Space through the testbed's real controllers.
	bus.Bus
	// id selects the Space whose body mount waits for release.
	id string
	// release admits the selected Space's mount.
	release <-chan struct{}
	// mounts counts actual body mount requests made by the Session projection.
	mounts atomic.Int64
}

// AddDirective holds the selected body mount before it can emit any result.
func (g *resourcesListMountGate) AddDirective(ctx context.Context, dir directive.Directive, cb directive.ReferenceHandler) (directive.Instance, directive.Reference, error) {
	// Count the Session projection's body mount requests.
	if _, ok := dir.(sobject.MountSharedObjectBody); ok {
		g.mounts.Add(1)
	}

	// Gate the chosen Space, allowing stream cancellation to stop the wait.
	if mount, ok := dir.(sobject.MountSharedObjectBody); ok && mount.MountSharedObjectBodyRef().GetProviderResourceRef().GetId() == g.id {
		select {
		case <-g.release:
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		}
	}

	// Delegate to the existing testbed mount controllers.
	return g.Bus.AddDirective(ctx, dir, cb)
}

var _ bus.Bus = (*resourcesListMountGate)(nil)
