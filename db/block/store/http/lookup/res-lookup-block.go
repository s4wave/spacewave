package block_store_http_lookup

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/dex"
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
	handler.ClearValues()
	store, err := r.c.GetBlockStore(ctx)
	if err != nil {
		return err
	}
	val, err := dex.ReadLookupBlockFromNetworkValue(ctx, store, r.d.LookupBlockFromNetworkRef())
	if err != nil {
		_, _ = handler.AddValue(dex.NewLookupBlockFromNetworkValue(nil, err))
		return err
	}
	if len(val.GetData()) != 0 || !r.c.conf.GetSkipNotFound() {
		_, _ = handler.AddValue(val)
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = (*lookupBlockFromNetworkResolver)(nil)
