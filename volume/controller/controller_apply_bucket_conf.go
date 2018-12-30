package volume_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/volume"
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
	var vol volume.Volume
	select {
	case vb := <-o.c.volumeCh:
		o.c.volumeCh <- vb
		vol = vb.vol
	case <-ctx.Done():
		return ctx.Err()
	}

	if !checkApplyBucketConfMatchesVolume(o.dir, vol) {
		return nil
	}

	o.mtx.Lock()
	defer o.mtx.Unlock()

	if o.applied {
		return nil
	}

	ts := timestamp.Now()
	var errStr string
	updated, prev, curr, err := vol.PutBucketConfig(o.dir.ApplyBucketConfigBucketConf())
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

	o.applied = true
	if updated && curr.GetId() != "" {
		o.c.bucketMtx.Lock()
		o.c.flushBucketHandle(curr.GetId())
		o.c.bucketMtx.Unlock()
	}
	handler.AddValue(&bucket.ApplyBucketConfigResult{
		VolumeId:      vol.GetID(),
		BucketId:      curr.GetId(),
		BucketConf:    curr,
		OldBucketConf: prev,
		Timestamp:     &ts,
		Updated:       updated,
		Error:         errStr,
	})
	return nil
}

// checkApplyBucketConfMatchesVolume checks if a applybucketconfig matches a volume
func checkApplyBucketConfMatchesVolume(dir bucket.ApplyBucketConfig, vol volume.Volume) bool {
	if volumeIDConstraint := dir.ApplyBucketConfigVolumeIDRe(); volumeIDConstraint != nil {
		return volumeIDConstraint.MatchString(vol.GetID())
	}

	// if bucket config does not already exist and no constraint
	// then do not apply a new config.
	c, _ := vol.GetLatestBucketConfig(dir.ApplyBucketConfigBucketConf().GetId())
	return c != nil
}

// resolveApplyBucketConf returns a resolver for looking up a volume.
func (c *Controller) resolveApplyBucketConf(
	ctx context.Context,
	di directive.Instance,
	dir bucket.ApplyBucketConfig,
) (directive.Resolver, error) {
	select {
	case vb := <-c.volumeCh:
		c.volumeCh <- vb
		if !checkApplyBucketConfMatchesVolume(dir, vb.vol) {
			return nil, nil
		}
	default:
	}

	// Return resolver.
	return &applyBucketConfigResolver{c: c, ctx: ctx, dir: dir}, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*applyBucketConfigResolver)(nil))
