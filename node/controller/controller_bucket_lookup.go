package node_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket/lookup"
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
		var valID uint32
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		subCtx, subCtxCancel := context.WithCancel(ctx)
		r.c.mtx.Lock()
		bh := r.c.buckets[bucketID]
		created := bh == nil
		if created {
			bh = newLoadedBucket(r.c, bucketID)
			r.c.buckets[bucketID] = bh
			for k := range r.c.volumes {
				bh.PushVolume(k)
			}
			r.c.procBucketWake()
		}
		refCb := func(s *loadedBucketState) {
			if valID != 0 {
				handler.RemoveValue(valID)
				valID = 0
			}
			if s == nil {
				subCtxCancel()
				return
			}
			var accepted bool
			valID, accepted = handler.AddValue(
				newBucketLookupHandle(bh, s),
			)
			if !accepted {
				valID = 0
			}
		}
		refID := bh.AddRef(refCb)
		r.c.mtx.Unlock()

		// the bucket handle is asserted in bh. the handle will concurrently
		// look up bucket config against volumes in Execute.
		<-subCtx.Done()
		subCtxCancel()
		bh.ClearRef(refID)
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*buildBucketLookupResolver)(nil))
