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
	dir bucket.ListBuckets
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *listBucketsResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var vol volume.Volume
	select {
	case vol = <-o.c.volumeCh:
		o.c.volumeCh <- vol
	case <-ctx.Done():
		return ctx.Err()
	}

	// TODO: watch for changes?

	volID := vol.GetID()
	if !checkListBucketsMatchesVolume(o.dir, vol) {
		return nil
	}

	if bucketID := o.dir.ListBucketsBucketId(); bucketID != "" {
		bc, err := vol.GetBucketInfo(bucketID)
		if err != nil || bc == nil {
			return err
		}
		bc.VolumeId = volID
		handler.AddValue(bc)
		return nil
	}

	bi, err := vol.ListBucketInfo(nil)
	if err != nil {
		return err
	}
	for _, iv := range bi {
		iv.VolumeId = volID
		handler.AddValue(iv)
	}

	return nil
}

// checkListBucketsMatchesVolume checks if a ListBuckets matches a volume
func checkListBucketsMatchesVolume(dir bucket.ListBuckets, vol volume.Volume) bool {
	if volumeRe := dir.ListBucketsVolumeIDRe(); volumeRe != nil {
		if !volumeRe.MatchString(vol.GetID()) {
			return false
		}
	}

	return true
}

// resolveListBuckets returns a resolver for listing buckets.
func (c *Controller) resolveListBuckets(
	ctx context.Context,
	di directive.Instance,
	dir bucket.ListBuckets,
) (directive.Resolver, error) {
	select {
	case vol := <-c.volumeCh:
		c.volumeCh <- vol
		if !checkListBucketsMatchesVolume(dir, vol) {
			return nil, nil
		}
	default:
	}

	// Return resolver.
	return &listBucketsResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*listBucketsResolver)(nil))
