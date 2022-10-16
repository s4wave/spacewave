package volume_rpc_client

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
)

// lookupVolumeResolver resolves LookupVolume directives
type lookupVolumeResolver struct {
	c   *ProxyVolumeController
	ctx context.Context
	dir volume.LookupVolume
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *lookupVolumeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	if !volume.CheckLookupMatchesVolume(o.dir, vol, o.c.volumeIDAlias) {
		return nil
	}

	handler.AddValue(vol)
	return nil
}

// resolveLookupVolume returns a resolver for looking up a volume.
func (c *ProxyVolumeController) resolveLookupVolume(
	ctx context.Context,
	di directive.Instance,
	dir volume.LookupVolume,
) (directive.Resolver, error) {
	// check if we can immediately reject this directive
	if vb := c.volume.GetValue(); vb != nil {
		if !volume.CheckLookupMatchesVolume(dir, vb, c.volumeIDAlias) {
			return nil, nil
		}
	}

	// Return resolver.
	return &lookupVolumeResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupVolumeResolver)(nil))
