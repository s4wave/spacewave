package volume_controller

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
)

// listBucketsResolver resolves ListBuckets directives
type listBucketsResolver struct {
	c   *Controller
	dir volume.ListBuckets
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *listBucketsResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Wait for the volume whose bucket information will be published.
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	// Require the bucket listing directive to match this volume.
	if !checkListBucketsMatchesVolume(o.dir, vol.GetID(), o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	// Publish bucket information together with its volume information.
	addValue := func(bc *bucket.BucketInfo) error {
		vi, err := volume.NewVolumeInfo(
			ctx,
			o.c.GetControllerInfo(),
			vol,
		)
		if err != nil {
			return err
		}
		handler.AddValue(&volume.VolumeBucketInfo{
			BucketInfo: bc,
			VolumeInfo: vi,
		})
		return nil
	}

	// Publish the explicitly requested bucket when one is selected.
	if bucketID := o.dir.ListBucketsBucketId(); bucketID != "" {
		bc, err := vol.GetBucketInfo(ctx, bucketID)
		if err != nil || bc == nil {
			return err
		}
		return addValue(bc)
	}

	// List the volume buckets and publish their information.
	bi, err := vol.ListBucketInfo(ctx, nil)
	if err != nil {
		return err
	}
	for _, iv := range bi {
		if err := addValue(iv); err != nil {
			return err
		}
	}

	return nil
}

// checkListBucketsMatchesVolume checks if a ListBuckets matches the volume ID
// or one of its aliases.
func checkListBucketsMatchesVolume(dir volume.ListBuckets, volID string, alias []string) bool {
	// Match the volume ID or an alias against the pattern.
	if volumeRe := dir.ListBucketsVolumeIDRe(); volumeRe != nil {
		return volumeRe.MatchString(volID) || slices.ContainsFunc(alias, volumeRe.MatchString)
	}

	// Otherwise match any listed volume ID, or every volume when none is listed.
	volumeIDList := dir.ListBucketsVolumeIDList()
	return len(volumeIDList) == 0 || slices.ContainsFunc(volumeIDList, func(id string) bool {
		return id == volID || slices.Contains(alias, id)
	})
}

// resolveListBuckets returns a resolver for listing buckets.
func (c *Controller) resolveListBuckets(
	ctx context.Context,
	di directive.Instance,
	dir volume.ListBuckets,
) (directive.Resolver, error) {
	if vb := c.volume.GetValue(); vb != nil {
		if !checkListBucketsMatchesVolume(dir, vb.vol.GetID(), c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &listBucketsResolver{c: c, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = (*listBucketsResolver)(nil)
