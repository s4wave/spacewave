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
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}
	volID := vol.GetID()
	targetVolID := o.dir.BuildObjectStoreAPIVolumeID()
	if !volume.CheckIDMatchesAliases(targetVolID, volID, o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	for {
		handler.ClearValues()
		storeID := o.dir.BuildObjectStoreAPIStoreID()
		os, err := vol.OpenObjectStore(ctx, storeID)
		h := newObjectStoreHandle(ctx, o.c, vol, os, err, storeID)
		vid, accepted := handler.AddValue(h)
		if !accepted {
			return nil
		}
		select {
		case <-ctx.Done():
			h.ctxCancel()
			return ctx.Err()
		case <-h.GetContext().Done():
			h.ctxCancel()
		}
		handler.RemoveValue(vid)
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
	// check if we can immediately reject this directive
	if vb := c.volume.GetValue(); vb != nil {
		targetVolID := dir.BuildObjectStoreAPIVolumeID()
		if !volume.CheckIDMatchesAliases(targetVolID, vb.vol.GetID(), c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &buildObjectStoreAPIResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*buildObjectStoreAPIResolver)(nil))
