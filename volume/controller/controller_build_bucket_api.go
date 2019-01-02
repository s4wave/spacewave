package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
)

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
	var vol volume.Volume
	select {
	case vb := <-o.c.volumeCh:
		o.c.volumeCh <- vb
		vol = vb.vol
	case <-ctx.Done():
		return ctx.Err()
	}
	volID := vol.GetID()
	if volID != o.dir.BuildBucketAPIVolumeID() {
		return nil
	}

	for {
		h, err := o.c.BuildBucketAPI(o.ctx, o.dir.BuildBucketAPIBucketID())
		if err != nil {
			return err
		}
		vid, accepted := handler.AddValue(h)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-h.GetContext().Done():
		}
		if accepted {
			handler.RemoveValue(vid)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

// resolveBuildBucketAPI returns a resolver for building a bucket API handle.
func (c *Controller) resolveBuildBucketAPI(
	ctx context.Context,
	di directive.Instance,
	dir volume.BuildBucketAPI,
) (directive.Resolver, error) {
	select {
	case vol := <-c.volumeCh:
		c.volumeCh <- vol
		volumeID := vol.vol.GetID()
		if dir.BuildBucketAPIVolumeID() != volumeID {
			return nil, nil
		}
	default:
	}

	// Return resolver.
	return &buildBucketAPIResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*buildBucketAPIResolver)(nil))
