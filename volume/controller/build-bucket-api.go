package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
)

// buildBucketAPIResolver resolves BuildBucketAPI directives
type buildBucketAPIResolver struct {
	c   *Controller
	ctx context.Context
	dir bucket.BuildBucketAPI
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *buildBucketAPIResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	// remove any old values we pushed
	handler.ClearValues()

	// make sure the volume ID matches
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}
	volID := vol.GetID()
	targetVolumeID := o.dir.BuildBucketAPIStoreID()
	if targetVolumeID == "" || !volume.CheckIDMatchesAliases(targetVolumeID, volID, o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	// add reference to bucket
	bucketID := o.dir.BuildBucketAPIBucketID()
	ref, ht, _ := o.c.bucketHandles.AddKeyRef(bucketID)
	defer ref.Release()

	// wait for bucket api to be built
	var handle *bucketHandle
	for {
		handle, err = ht.handleCtr.WaitValueChange(ctx, handle, nil)
		if err != nil {
			return err
		}

		handler.ClearValues()
		if handle != nil {
			_, _ = handler.AddValue(handle)
		}
	}
}

// resolveBuildBucketAPI returns a resolver for building a bucket API handle.
func (c *Controller) resolveBuildBucketAPI(
	ctx context.Context,
	di directive.Instance,
	dir bucket.BuildBucketAPI,
) (directive.Resolver, error) {
	// check if we can immediately reject this directive.
	if vb := c.volume.GetValue(); vb != nil {
		vol := vb.vol
		volID := vol.GetID()
		targetVolumeID := dir.BuildBucketAPIStoreID()
		if targetVolumeID == "" || !volume.CheckIDMatchesAliases(targetVolumeID, volID, c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &buildBucketAPIResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*buildBucketAPIResolver)(nil))
