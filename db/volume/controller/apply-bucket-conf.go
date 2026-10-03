package volume_controller

import (
	"context"
	"errors"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
)

// applyBucketConfigResolver resolves ApplyBucketConfig directives.
type applyBucketConfigResolver struct {
	c   *Controller
	dir bucket.ApplyBucketConfig

	mtx     sync.Mutex
	applied bool
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *applyBucketConfigResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Wait for the controlled volume before applying its bucket configuration.
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	// Require the bucket configuration directive to match this volume.
	if !bucket.CheckApplyBucketConfigMatchesVolume(o.dir, vol.GetID(), o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	// Serialize bucket configuration attempts through this resolver.
	o.mtx.Lock()
	defer o.mtx.Unlock()

	// Skip a bucket configuration already applied by this resolver.
	if o.applied {
		return nil
	}

	// Apply the bucket configuration and publish any storage failure.
	ts := timestamp.Now()
	updated, prev, curr, err := vol.ApplyBucketConfig(ctx, o.dir.ApplyBucketConfigBucketConf())
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		o.applied = true
		handler.AddValue(&bucket.ApplyBucketConfigResult{
			VolumeId:  vol.GetID(),
			BucketId:  o.dir.ApplyBucketConfigBucketConf().GetId(),
			Timestamp: ts,
			Error:     err.Error(),
		})
		return nil
	}

	// An unchanged bucket rejected by the Volume has no value.
	if !updated && curr.GetId() == "" {
		if prev != nil {
			curr = prev
		} else {
			return nil
		}
	}

	// Report the current bucket configuration without a previous update.
	if !updated {
		if curr == nil && prev != nil {
			curr = prev
		}
		prev = nil
	}

	// Refresh updated bucket handles and publish the configuration result.
	volID := vol.GetID()
	o.applied = true
	if updated {
		o.c.le.
			WithField("bucket-id", o.dir.ApplyBucketConfigBucketConf().GetId()).
			WithField("volume-id", volID).
			WithField("prev-bucket-rev", prev.GetRev()).
			WithField("bucket-rev", curr.GetRev()).
			Debug("updated bucket config")
		_ = o.c.restartBucketHandle(curr.GetId(), curr)
	}
	handler.AddValue(&bucket.ApplyBucketConfigResult{
		VolumeId:      volID,
		BucketId:      curr.GetId(),
		BucketConf:    curr,
		OldBucketConf: prev,
		Timestamp:     ts,
		Updated:       updated,
	})
	return nil
}

// resolveApplyBucketConf returns a resolver for looking up a volume.
func (c *Controller) resolveApplyBucketConf(
	ctx context.Context,
	di directive.Instance,
	dir bucket.ApplyBucketConfig,
) (directive.Resolver, error) {
	// Reject a directive that cannot match the current Volume.
	if vb := c.volume.GetValue(); vb != nil {
		if !bucket.CheckApplyBucketConfigMatchesVolume(dir, vb.vol.GetID(), c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &applyBucketConfigResolver{c: c, dir: dir}, nil
}

// _ asserts the resolver contract.
var _ directive.Resolver = (*applyBucketConfigResolver)(nil)
