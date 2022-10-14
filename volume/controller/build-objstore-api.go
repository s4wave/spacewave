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

// checkVolumeIDMatch checks if the volume ID matches the value or any alias.
// Returns true if the volume id target was empty
func checkVolumeIDMatch(targetVolID, volID string, alias []string) bool {
	if targetVolID == "" {
		return true
	}
	if volID == targetVolID {
		return true
	}
	for _, aliasID := range alias {
		if aliasID == targetVolID {
			return true
		}
	}
	return false
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
	if !checkVolumeIDMatch(targetVolID, volID, o.c.config.GetVolumeIdAlias()) {
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
	// check if we can immediately reject this directive
	if vb := c.volume.GetValue(); vb != nil {
		targetVolID := dir.BuildObjectStoreAPIVolumeID()
		if !checkVolumeIDMatch(targetVolID, vb.vol.GetID(), c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &buildObjectStoreAPIResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*buildObjectStoreAPIResolver)(nil))
