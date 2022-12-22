package volume_controller

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
)

// debounceBuildBucketAPI is the max rate to retry second time.
const debounceBuildBucketAPI = time.Millisecond * 500

// buildBucketAPIResolver resolves BuildBucketAPI directives
type buildBucketAPIResolver struct {
	c   *Controller
	ctx context.Context
	dir volume.BuildBucketAPI
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
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}
	volID := vol.GetID()
	targetVolumeID := o.dir.BuildBucketAPIVolumeID()
	if targetVolumeID == "" || !volume.CheckVolumeIDMatch(targetVolumeID, volID, o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	var prevTime time.Time
	for {
		handler.ClearValues()
		h, err := o.c.BuildBucketAPI(o.ctx, o.dir.BuildBucketAPIBucketID())
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		vid, accepted := handler.AddValue(h)
		if !accepted {
			h.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-h.GetContext().Done():
		}
		handler.RemoveValue(vid)
		select {
		case <-o.ctx.Done():
			return o.ctx.Err()
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Ensure we don't do this too frequently
			if sinceDur := time.Since(prevTime); sinceDur < debounceBuildBucketAPI {
				t := time.NewTimer(debounceBuildBucketAPI - sinceDur)
				select {
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-t.C:
				}
			}
			// Sometimes the volume cancels the bucket handle, we should re-try.
			o.c.le.Debugf("rebuilding canceled bucket handle: %s", o.dir.BuildBucketAPIBucketID())
			prevTime = time.Now()
		}
	}
}

// resolveBuildBucketAPI returns a resolver for building a bucket API handle.
func (c *Controller) resolveBuildBucketAPI(
	ctx context.Context,
	di directive.Instance,
	dir volume.BuildBucketAPI,
) (directive.Resolver, error) {
	// check if we can immediately reject this directive.
	if vb := c.volume.GetValue(); vb != nil {
		vol := vb.vol
		volID := vol.GetID()
		targetVolumeID := dir.BuildBucketAPIVolumeID()
		if targetVolumeID == "" || !volume.CheckVolumeIDMatch(targetVolumeID, volID, c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &buildBucketAPIResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*buildBucketAPIResolver)(nil))
