package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
)

// buildObjectStoreAPIResolver resolves BuildObjectStoreAPI directives
type buildObjectStoreAPIResolver struct {
	c   *Controller
	ctx context.Context
	dir volume.BuildObjectStoreAPI
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *buildObjectStoreAPIResolver) Resolve(
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
	if volID != o.dir.BuildObjectStoreAPIVolumeID() {
		return nil
	}

	for {
		storeID := o.dir.BuildObjectStoreAPIStoreID()
		os, err := vol.OpenObjectStore(ctx, storeID)
		h := newObjectStoreHandle(ctx, o.c, vol, os, err, storeID)
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

// resolveBuildObjectStoreAPI returns a resolver for building a object store API handle.
func (c *Controller) resolveBuildObjectStoreAPI(
	ctx context.Context,
	di directive.Instance,
	dir volume.BuildObjectStoreAPI,
) (directive.Resolver, error) {
	select {
	case vol := <-c.volumeCh:
		c.volumeCh <- vol
		volumeID := vol.vol.GetID()
		if dir.BuildObjectStoreAPIVolumeID() != volumeID {
			return nil, nil
		}
	default:
	}

	// Return resolver.
	return &buildObjectStoreAPIResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*buildObjectStoreAPIResolver)(nil))
