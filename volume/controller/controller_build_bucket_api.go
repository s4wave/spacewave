package volume_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
)

// buildBucketAPIResolver resolves BuildBucketAPI directives
type buildBucketAPIResolver struct {
	c   *Controller
	ctx context.Context
	dir bucket.BuildBucketAPI
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *buildBucketAPIResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	if vidRe := o.dir.BuildBucketAPIVolumeIDRe(); vidRe != nil {
		var vol volume.Volume
		select {
		case vb := <-o.c.volumeCh:
			o.c.volumeCh <- vb
			vol = vb.vol
		case <-ctx.Done():
			return ctx.Err()
		}

		if !vidRe.MatchString(vol.GetID()) {
			return nil
		}
	}

	var mtx sync.Mutex
	valsMap := make(map[string]uint32)
	return o.c.BuildBucketAPI(
		o.ctx,
		o.dir.BuildBucketAPIBucketID(),
		func(b bucket.Bucket, added bool) {
			bucketID := b.GetID()
			mtx.Lock()
			defer mtx.Unlock()

			if added {
				id, accepted := handler.AddValue(b)
				if accepted {
					valsMap[bucketID] = id
				}
			} else if vid, ok := valsMap[bucketID]; ok {
				handler.RemoveValue(vid)
				delete(valsMap, bucketID)
			}
		},
	)
}

// resolveBuildBucketAPI returns a resolver for building a bucket API handle.
func (c *Controller) resolveBuildBucketAPI(
	ctx context.Context,
	di directive.Instance,
	dir bucket.BuildBucketAPI,
) (directive.Resolver, error) {
	if volRe := dir.BuildBucketAPIVolumeIDRe(); volRe != nil {
		select {
		case vol := <-c.volumeCh:
			c.volumeCh <- vol
			volumeID := vol.vol.GetID()
			if !volRe.MatchString(volumeID) {
				return nil, nil
			}
		default:
		}
	}

	// Return resolver.
	return &buildBucketAPIResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*buildBucketAPIResolver)(nil))
