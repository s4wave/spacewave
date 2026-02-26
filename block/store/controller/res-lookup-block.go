package block_store_controller

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/dex"
)

// lookupBlockFromNetworkResolver resolves LookupBlockFromNetwork
type lookupBlockFromNetworkResolver struct {
	c *Controller
	d dex.LookupBlockFromNetwork
}

// resolveLookupBlockFromNetwork resolves the LookupBlockFromNetwork directive.
func (c *Controller) resolveLookupBlockFromNetwork(
	ctx context.Context,
	di directive.Instance,
	dir dex.LookupBlockFromNetwork,
) ([]directive.Resolver, error) {
	lookupBucketID := dir.LookupBlockFromNetworkBucketId()
	if lookupBucketID == "" || !slices.Contains(c.bucketIDs, lookupBucketID) {
		return nil, nil
	}
	return directive.R(&lookupBlockFromNetworkResolver{
		c: c,
		d: dir,
	}, nil)
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *lookupBlockFromNetworkResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	store, storeRef, err := r.c.WaitBlockStore(ctx)
	if err != nil {
		return err
	}
	defer storeRef.Release()

	data, found, err := store.GetBlock(ctx, r.d.LookupBlockFromNetworkRef())
	if err != nil {
		return err
	}
	handler.ClearValues()
	if found || !r.c.skipNotFound || err != nil {
		val := dex.NewLookupBlockFromNetworkValue(data, err)
		_, _ = handler.AddValue(val)
	}
	return err
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupBlockFromNetworkResolver)(nil))
