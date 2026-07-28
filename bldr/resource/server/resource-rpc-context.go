package resource_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// resourceRPCContext owns resources created by one ResourceRpc invocation.
type resourceRPCContext struct {
	client   *RemoteResourceClient
	finished bool
}

func newResourceRPCContext(client *RemoteResourceClient) *resourceRPCContext {
	return &resourceRPCContext{client: client}
}

func (c *resourceRPCContext) Context() context.Context {
	return c.client.Context()
}

func (c *resourceRPCContext) AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error) {
	return c.client.addInvocationResource(c, mux, nil, releaseFn)
}

func (c *resourceRPCContext) AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error) {
	return c.client.addInvocationResource(c, mux, value, releaseFn)
}

func (c *resourceRPCContext) ReleaseResource(resourceID uint32) bool {
	return c.client.ReleaseResource(resourceID)
}

func (c *resourceRPCContext) GetResourceValue(resourceID uint32) (any, error) {
	return c.client.GetResourceValue(resourceID)
}

func (c *resourceRPCContext) GetAttachedResource(resourceID uint32) (srpc.Client, error) {
	return c.client.GetAttachedResource(resourceID)
}

func (c *resourceRPCContext) finish(success bool) {
	c.client.finishInvocation(c, success, true)
}

func (c *resourceRPCContext) finishLegacy(success bool) {
	c.client.finishInvocation(c, success, false)
}

// _ is a type assertion
var _ ResourceClientContext = (*resourceRPCContext)(nil)
