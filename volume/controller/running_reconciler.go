package volume_controller

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/mqueue"
	"github.com/aperturerobotics/hydra/reconciler"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// runningReconciler is a running reconciler instance.
type runningReconciler struct {
	le        *logrus.Entry
	ctx       context.Context
	cancel    context.CancelFunc
	bucketRef *keyed.KeyedRef[string, *bucketHandleTracker]
	bucketTrk *bucketHandleTracker
	pair      bucket_store.BucketReconcilerPair
	v         volume.Volume
	b         bus.Bus
	reqQueue  mqueue.Queue
}

// newRunningReconciler builds a new running reconciler.
func newRunningReconciler(
	ctx context.Context,
	le *logrus.Entry,
	bucketRef *keyed.KeyedRef[string, *bucketHandleTracker],
	bucketTrk *bucketHandleTracker,
	b bus.Bus,
	pair bucket_store.BucketReconcilerPair,
	v volume.Volume,
	reqQueue mqueue.Queue,
) *runningReconciler {
	rec := &runningReconciler{
		v:         v,
		b:         b,
		le:        le,
		bucketRef: bucketRef,
		bucketTrk: bucketTrk,
		pair:      pair,
		reqQueue:  reqQueue,
	}
	rec.ctx, rec.cancel = context.WithCancel(ctx)
	return rec
}

// executeReconciler executes the reconciler instance.
func (r *runningReconciler) executeReconciler() error {
	// wait for the bucket handle
	ctx := r.ctx
	bucketHandle, err := r.bucketTrk.handleCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// get the bucket config
	if !bucketHandle.GetExists() {
		// bucket does not exist
		r.le.
			WithField("reconciler-id", r.pair.ReconcilerID).
			Debug("bucket not found")
		return nil
	}

	bc := bucketHandle.GetBucketConfig()
	recs := bc.GetReconcilers()
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
			Debugf("bucket reconciler config not found, purging event queue")
		return r.v.DeleteReconcilerEventQueue(ctx, r.pair)
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

	recControllerCtr := ccontainer.NewCContainer[*reconciler.Controller](nil)
	_, dirRef, err := r.b.AddDirective(
		configset.NewApplyConfigSet(csf),
		bus.NewCallbackHandler(
			func(val directive.AttachedValue) {
				cs, csOk := val.GetValue().(configset.State)
				if !csOk {
					return
				}
				csci := cs.GetController()
				recCon, recConOk := csci.(reconciler.Controller)
				if recCon == nil || !recConOk {
					return
				}
				recControllerCtr.SetValue(&recCon)
			}, func(val directive.AttachedValue) {
				cs, csOk := val.GetValue().(configset.State)
				if !csOk {
					return
				}
				csci := cs.GetController()
				recCon, recConOk := csci.(reconciler.Controller)
				if recCon == nil || !recConOk {
					return
				}
				recControllerCtr.SwapValue(func(val *reconciler.Controller) *reconciler.Controller {
					if val != nil && *val == recCon {
						val = nil
					}
					return val
				})
			}, func() {
				recControllerCtr.SetValue(nil)
				r.release()
			}),
	)
	if err != nil {
		return err
	}
	defer dirRef.Release()

	// TODO: handle if bucketHandle is purged
	var handle reconciler.Handle = newReconcilerHandle(
		r.pair,
		bucketHandle,
		r.v,
		r.reqQueue,
	)

	var reconcilerCtrl *reconciler.Controller
	for {
		reconcilerCtrl, err = recControllerCtr.WaitValueChange(ctx, reconcilerCtrl, nil)
		if err != nil {
			return err
		}
		if reconcilerCtrl == nil {
			continue
		}
		con := (*reconcilerCtrl)
		con.PushReconcilerHandle(handle)
	}
}

// release releases the running reconciler.
func (r *runningReconciler) release() {
	r.cancel()
	r.bucketRef.Release()
}
