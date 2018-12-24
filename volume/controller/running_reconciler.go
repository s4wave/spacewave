package volume_controller

import (
	"context"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/store"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
	"strings"
)

// runningReconciler is a running reconciler instance.
type runningReconciler struct {
	le        *logrus.Entry
	ctx       context.Context
	ctxCancel context.CancelFunc
	pair      bucket_store.BucketReconcilerPair
	v         volume.Volume
	b         bus.Bus
}

// newRunningReconciler builds a new running reconciler.
func newRunningReconciler(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	pair bucket_store.BucketReconcilerPair,
	v volume.Volume,
) *runningReconciler {
	rec := &runningReconciler{pair: pair, v: v, b: b}
	rec.ctx, rec.ctxCancel = context.WithCancel(ctx)
	return rec
}

// Execute executes reconciler instance.
func (r *runningReconciler) Execute() error {
	// ctx := r.ctx
	// 1. Load the bucket configuration.
	latestBc, err := r.v.GetLatestBucketConfig(r.pair.BucketID)
	if err != nil {
		return err
	}
	if latestBc == nil {
		r.le.Debug("bucket config not found")
		// TODO: Decide how to fix this condition.
		// return r.v.DeleteReconcilerEventQueue(r.pair)
		return nil
	}

	// 2. Execute the reconciler.
	recs := latestBc.GetReconcilers()
	var bucketRec *bucket.ReconcilerConfig
	for _, rec := range recs {
		if rec.GetId() == r.pair.BucketID {
			bucketRec = rec
			break
		}
	}
	if bucketRec == nil {
		r.le.Debug("bucket reconciler config not found")
		// TODO: Decide how to fix this condition.
		// return r.v.DeleteReconcilerEventQueue(r.pair)
		return nil
	}

	bucketRecCc, err := bucketRec.GetController().Resolve(r.ctx, r.b)
	if err != nil {
		r.le.WithError(err).Warn("unable to resolve controller config")
		return err
	}

	// Issue a configset fragment.
	csf := configset.ConfigSet{}
	// Controller uuid is hydra/bucket/{bucket-id}/reconciler/{reconciler-id}
	uuid := strings.Join([]string{
		"hydra",
		"bucket",
		r.pair.BucketID,
		"reconciler",
		r.pair.ReconcilerID,
	}, "/")
	csf[uuid] = bucketRecCc

	_, dirRef, err := r.b.AddDirective(
		configset.NewApplyConfigSet(csf),
		bus.NewCallbackHandler(
			func(val directive.AttachedValue) {
				// TODO: on value added
				r.le.Debugf("controller value added w/ id %v", val.GetValueID())
			}, func(val directive.AttachedValue) {
				// TODO: on value removed
				r.le.Debugf("controller value removed w/ id %v", val.GetValueID())
			}, func() {
				r.le.Debugf("controller directive disposed")
				// TODO: on directive disposed
				r.ctxCancel()
			}),
	)
	if err != nil {
		return err
	}
	<-r.ctx.Done()
	dirRef.Release()
	return nil
}
