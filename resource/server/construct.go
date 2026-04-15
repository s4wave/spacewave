package resource_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// ConstructChildResource creates a sub-resource with a managed context.
//
// Extracts the ResourceClientContext from ctx, creates a sub-context derived
// from the client session (outlives the RPC call), calls buildFn with that
// context, and registers the resulting mux as a child resource.
//
// On buildFn error the sub-context is canceled automatically. On success the
// sub-context lives until the resource is released or the client disconnects.
// The releaseFn returned by buildFn runs before the sub-context is canceled
// (user cleanup has a live context on explicit release; on client disconnect
// the client context is already dead which is accepted as abnormal teardown).
func ConstructChildResource[T any](
	ctx context.Context,
	buildFn func(subCtx context.Context) (mux srpc.Invoker, result T, releaseFn func(), err error),
) (T, uint32, error) {
	var zero T

	client, err := MustGetResourceClientContext(ctx)
	if err != nil {
		return zero, 0, err
	}

	subCtx, subCancel := context.WithCancel(client.Context())

	mux, result, releaseFn, err := buildFn(subCtx)
	if err != nil {
		subCancel()
		return zero, 0, err
	}

	resourceID, err := client.AddResourceValue(mux, result, func() {
		if releaseFn != nil {
			releaseFn()
		}
		subCancel()
	})
	if err != nil {
		if releaseFn != nil {
			releaseFn()
		}
		subCancel()
		return zero, 0, err
	}

	return result, resourceID, nil
}
