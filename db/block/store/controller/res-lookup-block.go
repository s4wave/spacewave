package block_store_controller

import (
	"context"
	"slices"

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
	// Skip directives for buckets this controller does not serve.
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
	// Wait for the block store reference.
	store, storeRef, err := r.c.WaitBlockStore(ctx)
	if err != nil {
		return err
	}
	defer storeRef.Release()

	// Read the lookup value from the block store.
	val, err := dex.ReadLookupBlockFromNetworkValue(ctx, store, r.d.LookupBlockFromNetworkRef())
	if err != nil {
		return err
	}

	// Clear stale values and emit the value unless it is missing and skipped.
	handler.ClearValues()
	if len(val.GetData()) != 0 || !r.c.skipNotFound {
		_, _ = handler.AddValue(val)
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = (*lookupBlockFromNetworkResolver)(nil)
