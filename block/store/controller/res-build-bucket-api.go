package block_store_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/dex"
	"golang.org/x/exp/slices"
)

// buildBucketAPIResolver resolves BuildBucketAPI
type buildBucketAPIResolver struct {
	c *Controller
	d bucket.BuildBucketAPI
}

// resolveBuildBucketAPI resolves the BuildBucketAPI directive.
func (c *Controller) resolveBuildBucketAPI(
	ctx context.Context,
	di directive.Instance,
	dir bucket.BuildBucketAPI,
) ([]directive.Resolver, error) {
	storeID := dir.BuildBucketAPIStoreID()
	if storeID != "" && !slices.Contains(c.blockStoreIds, storeID) {
		return nil, nil
	}

	lookupBucketID := dir.BuildBucketAPIBucketID()
	if lookupBucketID == "" || !slices.Contains(c.bucketIDs, lookupBucketID) {
		return nil, nil
	}

	return directive.R(&buildBucketAPIResolver{
		c: c,
		d: dir,
	}, nil)
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *buildBucketAPIResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	store, storeRef, err := r.c.WaitBlockStore(ctx)
	if err != nil {
		return err
	}
	defer storeRef.Release()

	data, found, err := store.GetBlock(ctx, r.d.BuildBucketAPIRef())
	if err != nil {
		return err
	}
	handler.ClearValues()
	if found || !r.c.skipNotFound || err != nil {
		var val dex.BuildBucketAPIValue = dex.NewBuildBucketAPIValue(data, err)
		_, _ = handler.AddValue(val)
	}
	return err
}

// _ is a type assertion
var _ directive.Resolver = ((*buildBucketAPIResolver)(nil))
