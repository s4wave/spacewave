package volume_controller

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/reconciler"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// runningReconciler is a running reconciler instance.
type runningReconciler struct {
	le           *logrus.Entry
	ctx          context.Context
	ctxCancel    context.CancelFunc
	pair         bucket_store.BucketReconcilerPair
	bc           *bucket.Config
	v            volume.Volume
	b            bus.Bus
	reqQueue     mqueue.Queue
	bucketHandle volume.BucketHandle
}

// newRunningReconciler builds a new running reconciler.
func newRunningReconciler(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	bc *bucket.Config,
	pair bucket_store.BucketReconcilerPair,
	v volume.Volume,
	reqQueue mqueue.Queue,
	bucketHandle volume.BucketHandle,
) *runningReconciler {
	rec := &runningReconciler{
		v:            v,
		b:            b,
		le:           le,
		bc:           bc,
		pair:         pair,
		reqQueue:     reqQueue,
		bucketHandle: bucketHandle,
	}
	rec.ctx, rec.ctxCancel = context.WithCancel(ctx)
	return rec
}

// Execute executes reconciler instance.
func (r *runningReconciler) Execute() error {
	recs := r.bc.GetReconcilers()
	var bucketRec *bucket.ReconcilerConfig
	for _, rec := range recs {
		if rec.GetId() == r.pair.ReconcilerID {
			bucketRec = rec
			break
		}
	}
	if bucketRec == nil {
		r.le.
			WithField("reconciler-id", r.pair.ReconcilerID).
			Debugf("bucket reconciler config not found")
		// TODO: Decide how to fix this condition.
		// return r.v.DeleteReconcilerEventQueue(r.pair)
		return nil
	}

	bucketRecCc, err := bucketRec.GetController().Resolve(r.ctx, r.b)
	if err != nil {
		if err != context.Canceled {
			r.le.WithError(err).Warn("unable to resolve controller config")
		}
		return err
	}

	// Ensure the config is a reconciler config.
	recConf, recConfOk := bucketRecCc.GetConfig().(reconciler.Config)
	if !recConfOk {
		err = errors.Errorf(
			"not a reconciler config: %s",
			bucketRecCc.GetConfig().GetConfigID(),
		)
		r.le.WithError(err).Warn("invalid config")
		return err
	}

	recConf.SetBucketId(r.pair.BucketID)
	recConf.SetReconcilerId(r.pair.ReconcilerID)
	recConf.SetVolumeId(r.v.GetID())

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

	recControllerCh := make(chan reconciler.Controller, 5)
	_, dirRef, err := r.b.AddDirective(
		configset.NewApplyConfigSet(csf),
		bus.NewCallbackHandler(
			func(val directive.AttachedValue) {
				cs, csOk := val.GetValue().(configset.State)
				csci := cs.GetController()
				recCon, recConOk := csci.(reconciler.Controller)
				r.le.Debugf(
					"controller value added w/ id %v csOk(%v) val(%#v) recConOk(%v) recCon(%#v)",
					val.GetValueID(),
					csOk,
					cs,
					recConOk,
					recCon,
				)
				if !csOk || csci == nil || !recConOk {
					return
				}
				// push the event queue and volume and bucket references to the
				// controller. do not use an additional directive to do the
				// lookup.
			RecPushLoop:
				for {
					select {
					case recControllerCh <- recCon:
						break RecPushLoop
					default:
					}
					select {
					case <-recControllerCh:
					default:
					}
				}
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

	var handle reconciler.Handle = newReconcilerHandle(
		r.ctx,
		r.ctxCancel,
		r.pair,
		r.bucketHandle,
		r.reqQueue,
	)

	// Wait for reconciler components to expire and call handleCtxCancel
	// Also, when the reconciler controller is created, push the handle.
RecConLoop:
	for {
		select {
		case <-r.bucketHandle.GetContext().Done():
			break RecConLoop
		case <-r.ctx.Done():
			break RecConLoop
		case recCon := <-recControllerCh:
			recCon.PushReconcilerHandle(handle)
		}
	}

	dirRef.Release()
	return nil
}
