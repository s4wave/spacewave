package volume_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
)

// listBucketsResolver resolves ListBuckets directives
type listBucketsResolver struct {
	c   *Controller
	ctx context.Context
	dir volume.ListBuckets
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *listBucketsResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	if !checkListBucketsMatchesVolume(o.dir, vol, o.c.config.GetVolumeIdAlias()) {
		return nil
	}

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
	if bucketID := o.dir.ListBucketsBucketId(); bucketID != "" {
		bc, err := vol.GetBucketInfo(bucketID)
		if err != nil || bc == nil {
			return err
		}
		if err := addValue(bc); err != nil {
			return err
		}
	}

	bi, err := vol.ListBucketInfo(nil)
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

// checkListBucketsMatchesVolume checks if a ListBuckets matches a volume
func checkListBucketsMatchesVolume(dir volume.ListBuckets, vol volume.Volume, alias []string) bool {
	if volumeRe := dir.ListBucketsVolumeIDRe(); volumeRe != nil {
		volID := vol.GetID()
		if volumeRe.MatchString(volID) {
			return true
		}
		for _, aliasID := range alias {
			if volumeRe.MatchString(aliasID) {
				return true
			}
		}
		return false
	}

	return true
}

// resolveListBuckets returns a resolver for listing buckets.
func (c *Controller) resolveListBuckets(
	ctx context.Context,
	di directive.Instance,
	dir volume.ListBuckets,
) (directive.Resolver, error) {
	if vb := c.volume.GetValue(); vb != nil {
		if !checkListBucketsMatchesVolume(dir, vb.vol, c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &listBucketsResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*listBucketsResolver)(nil))
