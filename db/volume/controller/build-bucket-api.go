package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
)

// buildBucketAPIResolver resolves BuildBucketAPI directives.
type buildBucketAPIResolver struct {
	// buckets retains active bucket resolution routines.
	buckets *keyed.KeyedRefCount[string, *bucketHandleTracker]
	// waitVolume waits for the controlled volume.
	waitVolume func(context.Context) (volume.Volume, error)
	// aliases contains the configured volume aliases.
	aliases []string
	// dir is the directive being resolved.
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
	// Remove any old values the resolver pushed.
	handler.ClearValues()

	// Wait for the volume before matching the requested bucket API.
	vol, err := o.waitVolume(ctx)
	if err != nil {
		return err
	}

	// Require an explicit target matching the volume or its aliases.
	volID := vol.GetID()
	targetVolumeID := o.dir.BuildBucketAPIStoreID()
	if targetVolumeID == "" || !volume.CheckIDMatchesAliases(targetVolumeID, volID, o.aliases) {
		return nil
	}

	// Add a reference to the bucket.
	bucketID := o.dir.BuildBucketAPIBucketID()
	ref, ht, _ := o.buckets.AddKeyRef(bucketID)
	defer ref.Release()

	// Publish each bucket resolution while the directive retains its tracker.
	return ht.publish(ctx, handler)
}

// resolveBuildBucketAPI returns a resolver for building a bucket API handle.
func (c *Controller) resolveBuildBucketAPI(
	ctx context.Context,
	di directive.Instance,
	dir bucket.BuildBucketAPI,
) (directive.Resolver, error) {
	// Check if the controller can immediately reject this directive.
	if vb := c.volume.GetValue(); vb != nil {
		vol := vb.vol
		// Require an explicit target matching the volume or its aliases.
		volID := vol.GetID()
		targetVolumeID := dir.BuildBucketAPIStoreID()
		if targetVolumeID == "" || !volume.CheckIDMatchesAliases(targetVolumeID, volID, c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return the resolver.
	return &buildBucketAPIResolver{
		waitVolume: c.GetVolume,
		aliases:    c.config.GetVolumeIdAlias(),
		dir:        dir,
		buckets:    c.bucketHandles,
	}, nil
}

// _ is a type assertion
var _ directive.Resolver = (*buildBucketAPIResolver)(nil)
