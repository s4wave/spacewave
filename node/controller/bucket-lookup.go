package node_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
)

// buildBucketLookupResolver resolves node.BuildBucketLookup
type buildBucketLookupResolver struct {
	c *Controller
	d bucket_lookup.BuildBucketLookup
}

func newBuildBucketLookupResolver(
	c *Controller,
	d bucket_lookup.BuildBucketLookup,
) *buildBucketLookupResolver {
	return &buildBucketLookupResolver{
		c: c,
		d: d,
	}
}

// resolveBuildBucketLookup resolves the node.BuildBucketLookup directive.
func (c *Controller) resolveBuildBucketLookup(
	ctx context.Context,
	di directive.Instance,
	d bucket_lookup.BuildBucketLookup,
) (directive.Resolver, error) {
	return newBuildBucketLookupResolver(c, d), nil
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *buildBucketLookupResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	bucketID := r.d.BuildBucketLookupBucketID()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		r.c.mtx.Lock()
		ref, bh, existed := r.c.buckets.AddKeyRef(bucketID)
		if !existed {
			// add initial volume set
			for k := range r.c.volumes {
				bh.PushVolume(k, false)
			}
		}
		stateCtr := bh.stateCtr
		r.c.mtx.Unlock()

		var currState *loadedBucketState
		for {
			state, err := stateCtr.WaitValueChange(ctx, currState, nil)
			if err != nil {
				// note: returns error only if context canceled
				break
			}
			currState = state
			handler.ClearValues()
			if currState == nil {
				continue
			}
			if currState.disposed {
				break
			}
			_, _ = handler.AddValue(newBucketLookupHandle(bh, currState))
		}

		handler.ClearValues()
		ref.Release()
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*buildBucketLookupResolver)(nil))
