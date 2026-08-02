package resource_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/bldr/resource"
)

// ResourceClientContext is the value attached to a Context containing
// information about the Resource RPC request.
type ResourceClientContext interface {
	// Context returns the lifetime context of the Resource RPC request.
	Context() context.Context
	// AddResource adds a child resource with a release callback.
	AddResource(mux srpc.Invoker, releaseFn func()) (uint32, error)
	// AddResourceValue adds a child resource with an optional typed value.
	AddResourceValue(mux srpc.Invoker, value any, releaseFn func()) (uint32, error)
	// ReleaseResource releases a child resource owned by this context.
	ReleaseResource(resourceID uint32) bool
	// GetResourceValue returns the typed value for a resource.
	GetResourceValue(resourceID uint32) (any, error)
	// GetAttachedResource returns a raw attached SRPC client.
	GetAttachedResource(id uint32) (srpc.Client, error)
}

// mountedStreamContextKey is the context key used for WithValue.
type mountedStreamContextKey struct{}

// WithResourceClientContext attaches a ResourceClientContext to a Context.
func WithResourceClientContext(ctx context.Context, msc ResourceClientContext) context.Context {
	return context.WithValue(ctx, mountedStreamContextKey{}, msc)
}

// GetResourceClientContext returns the ResourceClientContext from the Context or nil if unset.
func GetResourceClientContext(ctx context.Context) ResourceClientContext {
	val := ctx.Value(mountedStreamContextKey{})
	msc, ok := val.(ResourceClientContext)
	if !ok || msc == nil {
		return nil
	}
	return msc
}

// MustGetResourceClientContext returns the ResourceClientContext from the Context or an error if unset.
func MustGetResourceClientContext(ctx context.Context) (ResourceClientContext, error) {
	msc := GetResourceClientContext(ctx)
	if msc == nil {
		return nil, resource.ErrNoResourceClientContext
	}
	return msc, nil
}
