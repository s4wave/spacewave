package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/volume"
)

// lookupBlockStoreResolver resolves LookupBlockStore directives
type lookupBlockStoreResolver struct {
	c   *Controller
	ctx context.Context
	dir block_store.LookupBlockStore
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *lookupBlockStoreResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	if !volume.CheckLookupBlockStoreMatchesVolume(o.dir, vol, o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	handler.AddValue(vol)
	return nil
}

// resolveLookupBlockStore returns a resolver for looking up a volume.
func (c *Controller) resolveLookupBlockStore(
	ctx context.Context,
	di directive.Instance,
	dir block_store.LookupBlockStore,
) (directive.Resolver, error) {
	// check if we can immediately reject this directive
	if c.config.GetDisableLookupBlockStore() {
		return nil, nil
	}

	// check volume matches
	if vb := c.volume.GetValue(); vb != nil && vb.vol != nil {
		if !volume.CheckLookupBlockStoreMatchesVolume(dir, vb.vol, c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &lookupBlockStoreResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupBlockStoreResolver)(nil))
