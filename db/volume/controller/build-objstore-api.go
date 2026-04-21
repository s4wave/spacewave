package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/volume"
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

	storeID := o.dir.BuildObjectStoreAPIStoreID()
	resolve := directive.NewAccessResolver(func(ctx context.Context, released func()) (volume.BuildObjectStoreAPIValue, func(), error) {
		objStore, rel, err := vol.AccessObjectStore(ctx, storeID, released)
		if err != nil {
			return nil, rel, err
		}

		return newObjectStoreHandle(o.c, vol, objStore, storeID), rel, nil
	})
	return resolve.Resolve(ctx, handler)
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
