package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/volume"
)

// lookupVolumeResolver resolves LookupVolume directives
type lookupVolumeResolver struct {
	c   *Controller
	ctx context.Context
	dir volume.LookupVolume
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *lookupVolumeResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var vol volume.Volume
	select {
	case vb := <-o.c.volumeCh:
		o.c.volumeCh <- vb
		vol = vb.vol
	case <-ctx.Done():
		return ctx.Err()
	}

	if !checkLookupMatchesVolume(o.dir, vol, o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	handler.AddValue(vol)
	return nil
}

// checkLookupMatchesVolume checks if a lookupvolume matches a volume.
// only checks if there are any constraints set on the directive (not ID).
func checkLookupMatchesVolume(dir volume.LookupVolume, vol volume.Volume, alias []string) bool {
	if peerIDConstraint := dir.LookupVolumePeerIDConstraint(); len(peerIDConstraint) != 0 {
		if vol.GetPeerID() != peerIDConstraint {
			return false
		}
	}
	if !checkVolumeIDMatch(dir.LookupVolumeID(), vol.GetID(), alias) {
		return false
	}

	return true
}

// resolveLookupVolume returns a resolver for looking up a volume.
func (c *Controller) resolveLookupVolume(
	ctx context.Context,
	di directive.Instance,
	dir volume.LookupVolume,
) (directive.Resolver, error) {
	select {
	case vb := <-c.volumeCh:
		c.volumeCh <- vb
		if !checkLookupMatchesVolume(dir, vb.vol, c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	default:
	}

	// Return resolver.
	return &lookupVolumeResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupVolumeResolver)(nil))
