package volume_controller

import (
	"context"

	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/store"
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
	filledQueues, err := v.ListFilledReconcilerEventQueues()
	if err != nil {
		return err
	}

	c.reconcilersMtx.Lock()
	defer c.reconcilersMtx.Unlock()

	for _, q := range filledQueues {
		bc, err := v.GetLatestBucketConfig(q.BucketID)
		if err != nil {
			c.le.WithError(err).Warn("unable to lookup bucket config")
			continue
		}
		c.wakeReconcilerQueue(ctx, v, bc, q, nil)
	}

	return nil
}

// bucketLogger returns the logger for a bucket id
func bucketLogger(le *logrus.Entry, id string) *logrus.Entry {
	return le.WithField("bucket-id", id)
}

// wakeReconcilerQueue attempts to start the process of waking a reconciler
// expects reconcilersMtx to be locked by the caller.
func (c *Controller) wakeReconcilerQueue(
	ctx context.Context,
	v volume.Volume,
	bc *bucket.Config,
	pair bucket_store.BucketReconcilerPair,
	event []byte,
) (_ mqueue.Queue, rerr error) {
	bucketID := pair.BucketID
	le := bucketLogger(c.le, bucketID)
	defer func() {
		if rerr != nil {
			le.
				WithError(rerr).
				Warn("cannot wake reconciler queue")
		}
	}()

	if pair.BucketID != bc.GetId() {
		return nil, errors.New("pair id does not match bucket config id")
	}

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

	var nbh *bucketHandle
	cnbh := func() *bucketHandle {
		return newBucketHandle(ctx, c, v, bc)
	}
	c.bucketMtx.Lock()
	if e, ok := c.bucketHandles[pair.BucketID]; ok {
		if e.bucketConf.GetVersion() < bc.GetVersion() {
			nbh = cnbh()
		} else {
			nbh = e
		}
	} else {
		nbh = cnbh()
		c.bucketHandles[bc.GetId()] = nbh
	}
	atth := newAttachedBucketHandle(ctx, nbh)
	c.bucketMtx.Unlock()

	rr := newRunningReconciler(ctx, le, c.bus, bc, pair, v, eq, atth)
	c.startRunningReconciler(le, pair, rr)
	return eq, nil
}

// startRunningReconciler executes a running reconciler
// expects reconcilersMtx to be locked by the caller.
func (c *Controller) startRunningReconciler(
	le *logrus.Entry,
	pair bucket_store.BucketReconcilerPair,
	rr *runningReconciler,
) {
	var e *runningReconciler
	var ok bool
	e, ok = c.reconcilers[pair]
	c.reconcilers[pair] = rr
	if ok {
		if e == rr {
			return
		}
		if e != nil {
			e.ctxCancel()
		}
	}
	if rr != nil {
		go func() {
			if err := rr.Execute(); err != nil && err != context.Canceled {
				le.
					WithError(err).
					Warn("reconciler exited with error")
			}
			rr.ctxCancel()
			rr.bucketHandle.Close()
			c.reconcilersMtx.Lock()
			if v, ok := c.reconcilers[pair]; ok && v == rr {
				delete(c.reconcilers, pair)
			}
			c.reconcilersMtx.Unlock()
		}()
	}
}

// pushEventToReconcilers pushes an event to all running reconcilers.
// wakes reconcilers
// expects reconcilersMtx to NOT BE LOCKED by the caller.
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
		c.reconcilersMtx.Lock()
		_, err = c.wakeReconcilerQueue(ctx, vol, bucketConf, pair, ed)
		c.reconcilersMtx.Unlock()
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
