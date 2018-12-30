package volume_controller

import (
	"context"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/store"
	"github.com/aperturerobotics/hydra/store/mqueue"
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
		c.wakeReconcilerQueue(ctx, v, bc, q)
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
) (eq mqueue.Queue, err error) {
	bucketID := pair.BucketID
	le := bucketLogger(c.le, bucketID)
	defer func() {
		if err != nil {
			le.
				WithError(err).
				Warn("cannot wake reconciler queue")
		}
	}()

	// reconciler instance already exists
	if _, ok := c.reconcilers[pair]; ok {
		return nil, nil
	}

	eq, err = v.GetReconcilerEventQueue(pair)
	if err != nil {
		return nil, errors.Wrap(err, "get reconciler event queue")
	}

	nbh := newBucketHandle(ctx, c, v, bc)
	c.bucketMtx.Lock()
	if e, ok := c.bucketHandles[bc.GetId()]; ok {
		if nbh.superceeds(e) {
			c.bucketHandles[bc.GetId()] = nbh
			e.ctxCancel()
		} else {
			nbh.ctxCancel()
			nbh = e
		}
	} else {
		c.bucketHandles[bc.GetId()] = nbh
	}
	defer nbh.startOperation().release()
	c.bucketMtx.Unlock()

	rr := newRunningReconciler(ctx, le, c.bus, bc, pair, v, eq, nbh)
	c.reconcilers[pair] = rr
	go func() {
		if err := rr.Execute(); err != nil && err != context.Canceled {
			le.
				WithError(err).
				Warn("reconciler exited with error")
		}
		c.reconcilersMtx.Lock()
		if v, ok := c.reconcilers[pair]; ok && v == rr {
			delete(c.reconcilers, pair)
		}
		c.reconcilersMtx.Unlock()
	}()

	return
}
