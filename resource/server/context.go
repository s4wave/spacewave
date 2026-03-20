package resource_server

import (
	"context"

	"github.com/aperturerobotics/bldr/resource"
)

// ResourceClientContext is the value attached to a Context containing
// information about the Resource RPC request.
type ResourceClientContext = *RemoteResourceClient

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
