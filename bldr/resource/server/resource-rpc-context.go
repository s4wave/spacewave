package resource_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// resourceRPCContext carries parent and invocation metadata for resources
// created during one ResourceRpc call. Adopt and Release controls determine
// each child's lifetime after the call returns.
type resourceRPCContext struct {
	client           *RemoteResourceClient
	parentResourceID uint32
	serviceID        string
	methodID         string
}

func newResourceRPCContext(
	client *RemoteResourceClient,
	parentResourceID uint32,
	serviceID string,
	methodID string,
) *resourceRPCContext {
	return &resourceRPCContext{
		client:           client,
		parentResourceID: parentResourceID,
		serviceID:        serviceID,
		methodID:         methodID,
	}
}

// Context returns the ResourceClient generation context.
func (c *resourceRPCContext) Context() context.Context { return c.client.Context() }

// AddResource adds a pending invocation child.
func (c *resourceRPCContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.client.addInvocationResource(c.parentResourceID, c.serviceID, c.methodID, mux, nil, releaseFn)
}

// AddResourceValue adds a pending invocation child with an in-process value.
func (c *resourceRPCContext) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return c.client.addInvocationResource(c.parentResourceID, c.serviceID, c.methodID, mux, value, releaseFn)
}

// ReleaseResource releases a resource from the server side.
func (c *resourceRPCContext) ReleaseResource(resourceID uint32) bool {
	return c.client.ReleaseResource(resourceID)
}

// GetResourceValue returns an in-process resource value.
func (c *resourceRPCContext) GetResourceValue(resourceID uint32) (any, error) {
	return c.client.GetResourceValue(resourceID)
}

// GetAttachedResource returns a client-published resource.
func (c *resourceRPCContext) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	return c.client.GetAttachedResource(resourceID)
}

// _ is a type assertion
var _ ResourceClientContext = (*resourceRPCContext)(nil)
