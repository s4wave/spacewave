package volume_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/timestamp"
)

// applyBucketConfigResolver resolves ApplyBucketConfig directives
type applyBucketConfigResolver struct {
	c   *Controller
	ctx context.Context
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
	vol, err := o.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	if !bucket.CheckApplyBucketConfigMatchesVolume(o.dir, vol.GetID(), o.c.config.GetVolumeIdAlias()) {
		return nil
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	if o.applied {
		return nil
	}

	ts := timestamp.Now()
	var errStr string
	updated, prev, curr, err := vol.ApplyBucketConfig(o.dir.ApplyBucketConfigBucketConf())
	if err != nil {
		if err == context.Canceled {
			return err
		}
		errStr = err.Error()
	}

	// no effect and no bucket data -> no value
	if !updated && curr.GetId() == "" {
		if prev != nil {
			curr = prev
		} else {
			return nil
		}
	}

	if !updated {
		if curr == nil && prev != nil {
			curr = prev
		}
		prev = nil
	}

	volID := vol.GetID()
	o.applied = true
	if updated {
		o.c.le.
			WithField("bucket-id", o.dir.ApplyBucketConfigBucketConf().GetId()).
			WithField("volume-id", volID).
			WithField("prev-bucket-rev", prev.GetRev()).
			WithField("bucket-rev", curr.GetRev()).
			Debug("updated bucket config")
		o.c.mtx.Lock()
		_ = o.c.restartBucketHandle(curr.GetId(), curr)
		o.c.mtx.Unlock()
	}
	handler.AddValue(&bucket.ApplyBucketConfigResult{
		VolumeId:      volID,
		BucketId:      curr.GetId(),
		BucketConf:    curr,
		OldBucketConf: prev,
		Timestamp:     &ts,
		Updated:       updated,
		Error:         errStr,
	})
	return nil
}

// resolveApplyBucketConf returns a resolver for looking up a volume.
func (c *Controller) resolveApplyBucketConf(
	ctx context.Context,
	di directive.Instance,
	dir bucket.ApplyBucketConfig,
) (directive.Resolver, error) {
	// check if we can immediately reject this directive or not
	if vb := c.volume.GetValue(); vb != nil {
		if !bucket.CheckApplyBucketConfigMatchesVolume(dir, vb.vol.GetID(), c.config.GetVolumeIdAlias()) {
			return nil, nil
		}
	}

	// Return resolver.
	return &applyBucketConfigResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*applyBucketConfigResolver)(nil))
