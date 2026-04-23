package volume_controller

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	"github.com/s4wave/spacewave/db/mqueue"
	"github.com/s4wave/spacewave/db/reconciler"
	volume "github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

var errBucketHandleChanged = errors.New("bucket handle changed")

// runningReconciler is a running reconciler instance.
type runningReconciler struct {
	c       *Controller
	le      *logrus.Entry
	pair    bucket_store.BucketReconcilerPair
	running atomic.Bool
}

// newRunningReconciler builds a new running reconciler.
func (c *Controller) newRunningReconciler(
	pair bucket_store.BucketReconcilerPair,
) (keyed.Routine, *runningReconciler) {
	rec := &runningReconciler{
		c:    c,
		le:   bucketLogger(c.le, pair.BucketID),
		pair: pair,
	}
	return rec.execute, rec
}

// executeReconciler executes the reconciler instance.
func (r *runningReconciler) execute(ctx context.Context) error {
	r.running.Store(true)
	defer r.running.Store(false)

	return r.executeReconciler(ctx)
}

// IsRunning indicates if the reconciler routine is currently active.
func (r *runningReconciler) IsRunning() bool {
	return r.running.Load()
}

// executeReconciler executes the reconciler instance.
func (r *runningReconciler) executeReconciler(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	vol, err := r.c.GetVolume(ctx)
	if err != nil {
		return err
	}

	reqQueue, err := vol.GetReconcilerEventQueue(ctx, r.pair)
	if err != nil {
		return errors.Wrap(err, "get reconciler event queue")
	}

	bucketRef, bucketTrk, _ := r.c.bucketHandles.AddKeyRef(r.pair.BucketID)
	defer bucketRef.Release()

	var bucketHandle *bucketHandle
	for {
		bucketHandle, err = bucketTrk.handleCtr.WaitValueChange(ctx, bucketHandle, nil)
		if err != nil {
			return err
		}
		if bucketHandle == nil {
			continue
		}
		err = r.executeBucketHandle(ctx, cancel, vol, reqQueue, bucketTrk, bucketHandle)
		if err == nil {
			return nil
		}
		if err == errBucketHandleChanged {
			continue
		}
		return err
	}
}

// executeBucketHandle executes one reconciler binding for a specific bucket handle snapshot.
func (r *runningReconciler) executeBucketHandle(
	ctx context.Context,
	cancel context.CancelFunc,
	vol volume.Volume,
	reqQueue mqueue.Queue,
	bucketTrk *bucketHandleTracker,
	bucketHandle *bucketHandle,
) error {
	if !bucketHandle.GetExists() {
		return r.cleanupMissingReconcilerState(ctx, vol, "bucket not found")
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
		return r.cleanupMissingReconcilerState(
			ctx,
			vol,
			"bucket reconciler config not found, purging event queue",
		)
	}

	errCh := make(chan error, 1)
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go r.watchBucketHandleChange(watchCtx, bucketTrk, bucketHandle, errCh)

	bucketRecCc, err := bucketRec.GetController().Resolve(ctx, r.c.bus)
	if err != nil {
		if err != context.Canceled {
			r.le.WithError(err).Warn("unable to resolve controller config")
		}
		return err
	}

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
	recConf.SetBlockStoreId(vol.GetID())

	csf := configset.ConfigSet{}
	uuid := strings.Join([]string{
		"hydra",
		"bucket",
		r.pair.BucketID,
		"reconciler",
		r.pair.ReconcilerID,
	}, "/")
	csf[uuid] = bucketRecCc

	recControllerCtr := ccontainer.NewCContainer[*reconciler.Controller](nil)
	_, dirRef, err := r.c.bus.AddDirective(
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
				cancel()
			}),
	)
	if err != nil {
		return err
	}
	defer dirRef.Release()

	handle := reconciler.Handle(newReconcilerHandle(
		r.pair,
		bucketHandle,
		vol,
		reqQueue,
	))

	var reconcilerCtrl *reconciler.Controller
	for {
		reconcilerCtrl, err = recControllerCtr.WaitValueChange(ctx, reconcilerCtrl, errCh)
		if err != nil {
			return err
		}
		if reconcilerCtrl == nil {
			continue
		}
		(*reconcilerCtrl).PushReconcilerHandle(handle)
	}
}

// cleanupMissingReconcilerState purges the queue and removes the desired key when
// the bucket or reconciler config no longer exists.
func (r *runningReconciler) cleanupMissingReconcilerState(
	ctx context.Context,
	vol volume.Volume,
	msg string,
) error {
	r.le.
		WithField("reconciler-id", r.pair.ReconcilerID).
		Debug(msg)
	if err := vol.DeleteReconcilerEventQueue(ctx, r.pair); err != nil {
		return err
	}
	r.c.removeReconcilerKey(r.pair)
	return nil
}

// watchBucketHandleChange reports when the bucket handle snapshot changes.
func (r *runningReconciler) watchBucketHandleChange(
	ctx context.Context,
	bucketTrk *bucketHandleTracker,
	bucketHandle *bucketHandle,
	errCh chan<- error,
) {
	_, err := bucketTrk.handleCtr.WaitValueChange(ctx, bucketHandle, nil)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		select {
		case errCh <- err:
		default:
		}
		return
	}
	select {
	case errCh <- errBucketHandleChanged:
	default:
	}
}
