package volume_controller

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	bucket_store "github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/mqueue"
	volume "github.com/aperturerobotics/hydra/volume"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// wakeFilledReconcilerQueues wakes all reconciler queues with at least one
// event.
func (c *Controller) wakeFilledReconcilerQueues(
	ctx context.Context,
	v volume.Volume,
) error {
	if c.config.GetDisableReconcilerQueues() {
		return volume.ErrReconcilerQueuesDisabled
	}
	filledQueues, err := v.ListFilledReconcilerEventQueues()
	if err != nil {
		return err
	}

	c.mtx.Lock()
	defer c.mtx.Unlock()

	for _, q := range filledQueues {
		_, _ = c.wakeReconcilerQueueLocked(ctx, v, q, nil)
	}

	return nil
}

// bucketLogger returns the logger for a bucket id
func bucketLogger(le *logrus.Entry, id string) *logrus.Entry {
	return le.WithField("bucket-id", id)
}

// wakeReconcilerQueue attempts to start the process of waking a reconciler
// expects mtx to be locked by the caller.
func (c *Controller) wakeReconcilerQueueLocked(
	ctx context.Context,
	v volume.Volume,
	pair bucket_store.BucketReconcilerPair,
	event []byte,
) (_ mqueue.Queue, rerr error) {
	if c.config.GetDisableReconcilerQueues() {
		return nil, volume.ErrReconcilerQueuesDisabled
	}

	bucketID := pair.BucketID
	le := bucketLogger(c.le, bucketID)
	defer func() {
		if rerr != nil {
			le.
				WithError(rerr).
				Warn("cannot wake reconciler queue")
		}
	}()

	// reconciler instance already exists
	if exist, ok := c.reconcilers[pair]; ok {
		if len(event) != 0 {
			_, err := exist.reqQueue.Push(event)
			if err != nil {
				return nil, err
			}
		}
		return exist.reqQueue, nil
	}

	eq, err := v.GetReconcilerEventQueue(pair)
	if err != nil {
		return nil, errors.Wrap(err, "get reconciler event queue")
	}

	if len(event) != 0 {
		_, err := eq.Push(event)
		if err != nil {
			return nil, err
		}
	}

	ref, nbh, _ := c.bucketHandles.AddKeyRef(pair.BucketID)
	rr := newRunningReconciler(ctx, le, ref, nbh, c.bus, pair, v, eq)
	c.execRunningReconcilerLocked(le, pair, rr)
	return eq, nil
}

// execRunningReconcilerLocked executes a running reconciler
// expects mtx to be locked by the caller.
func (c *Controller) execRunningReconcilerLocked(
	le *logrus.Entry,
	pair bucket_store.BucketReconcilerPair,
	rr *runningReconciler,
) {
	var e *runningReconciler
	var ok bool
	e, ok = c.reconcilers[pair]
	if ok && e != nil {
		if e == rr {
			return
		}
		e.release()
	}
	c.reconcilers[pair] = rr
	go func() {
		if err := rr.executeReconciler(); err != nil && err != context.Canceled {
			le.
				WithError(err).
				Warn("reconciler exited with error")
		}
		rr.release()
		c.mtx.Lock()
		if v, ok := c.reconcilers[pair]; ok && v == rr {
			delete(c.reconcilers, pair)
		}
		c.mtx.Unlock()
	}()
}

// pushEventToReconcilers pushes an event to all running reconcilers.
// wakes reconcilers
// expects mtx to NOT BE LOCKED by the caller.
func (c *Controller) pushEventToReconcilers(
	ctx context.Context,
	vol volume.Volume,
	bucketConf *bucket.Config,
	isPut bool,
	getEventData func() ([]byte, error),
) error {
	for _, rc := range bucketConf.GetReconcilers() {
		if isPut && rc.GetFilterPut() {
			continue
		}
		pair := bucket_store.BucketReconcilerPair{
			BucketID:     bucketConf.GetId(),
			ReconcilerID: rc.GetId(),
		}
		ed, err := getEventData()
		if err != nil {
			return err
		}
		c.mtx.Lock()
		_, err = c.wakeReconcilerQueueLocked(ctx, vol, pair, ed)
		c.mtx.Unlock()
		if err != nil {
			c.le.
				WithError(err).
				WithField("bucket-id", pair.BucketID).
				WithField("reconciler-id", pair.ReconcilerID).
				Warn("unable to push event to bucket reconciler queue")
			continue
		}
	}

	return nil
}
