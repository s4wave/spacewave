package resource_world

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

// resourceLease runs release after an owning resource and every resource
// registered through its RPCs, recursively, have been released.
//
// Adopted child resources can outlive their parent resource, so state shared
// with those children stays open until the last of them is released.
type resourceLease struct {
	refs    atomic.Int64
	release func()
}

// newResourceLease constructs a lease holding one reference for the owner.
func newResourceLease(release func()) *resourceLease {
	l := &resourceLease{release: release}
	l.refs.Store(1)
	return l
}

// acquire adds a reference to the lease.
func (l *resourceLease) acquire() {
	l.refs.Add(1)
}

// releaseRef drops a reference and runs release after the last one.
func (l *resourceLease) releaseRef() {
	if l.refs.Add(-1) == 0 {
		l.release()
	}
}

// wrapInvoker registers resources created by calls to inv under the lease.
func (l *resourceLease) wrapInvoker(inv srpc.Invoker) srpc.Invoker {
	return &resourceLeaseInvoker{lease: l, inv: inv}
}

// resourceLeaseInvoker attaches a lease to the resource context of each call.
type resourceLeaseInvoker struct {
	lease *resourceLease
	inv   srpc.Invoker
}

// InvokeMethod invokes the method with a leased resource context.
func (i *resourceLeaseInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	ctx := strm.Context()
	if resourceCtx := resource_server.GetResourceClientContext(ctx); resourceCtx != nil {
		leaseCtx := &resourceLeaseContext{ResourceClientContext: resourceCtx, lease: i.lease}
		strm = &resourceLeaseStream{
			Stream: strm,
			ctx:    resource_server.WithResourceClientContext(ctx, leaseCtx),
		}
	}
	return i.inv.InvokeMethod(serviceID, methodID, strm)
}

// resourceLeaseStream overrides the stream context.
type resourceLeaseStream struct {
	srpc.Stream
	ctx context.Context
}

// Context returns the leased stream context.
func (s *resourceLeaseStream) Context() context.Context {
	return s.ctx
}

// resourceLeaseContext holds a lease reference for each child resource.
type resourceLeaseContext struct {
	resource_server.ResourceClientContext
	lease *resourceLease
}

// AddResource adds a child resource holding a lease reference.
func (c *resourceLeaseContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.AddResourceValue(mux, nil, releaseFn)
}

// AddResourceValue adds a child resource value holding a lease reference.
func (c *resourceLeaseContext) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	c.lease.acquire()
	id, err := c.ResourceClientContext.AddResourceValue(c.lease.wrapInvoker(mux), value, func() {
		if releaseFn != nil {
			releaseFn()
		}
		c.lease.releaseRef()
	})
	if err != nil {
		c.lease.releaseRef()
		return 0, err
	}
	return id, nil
}

// _ is a type assertion
var (
	_ srpc.Invoker                          = (*resourceLeaseInvoker)(nil)
	_ resource_server.ResourceClientContext = (*resourceLeaseContext)(nil)
)
