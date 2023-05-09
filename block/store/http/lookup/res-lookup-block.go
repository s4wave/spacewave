package block_store_http_lookup

import (
	"context"

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
	matchBucketID := c.conf.GetBucketId()
	lookupBucketID := dir.LookupBlockFromNetworkBucketId()
	if lookupBucketID == "" || matchBucketID != lookupBucketID {
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
	store, err := r.c.GetHTTPStore(ctx)
	if err != nil {
		return err
	}

	data, found, err := store.GetBlock(r.d.LookupBlockFromNetworkRef())
	if err != nil {
		return err
	}
	handler.ClearValues()
	if found || !r.c.conf.GetSkipNotFound() || err != nil {
		var val dex.LookupBlockFromNetworkValue = dex.NewLookupBlockFromNetworkValue(data, err)
		_, _ = handler.AddValue(val)
	}
	return err
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupBlockFromNetworkResolver)(nil))
