package volume_controller

import (
	"context"
	"github.com/aperturerobotics/hydra/bucket/store"
	volume "github.com/aperturerobotics/hydra/volume"
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
		c.wakeReconcilerQueue(ctx, v, q)
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
	pair bucket_store.BucketReconcilerPair,
) (err error) {
	bucketID, reconcilerID := pair.BucketID, pair.ReconcilerID
	le := bucketLogger(c.le, bucketID)
	defer func() {
		if err != nil {
			le.
				WithError(err).
				Warn("cannot wake reconciler queue")
		}
	}()

	// reconciler instance already exists
	for _, rec := range c.reconcilers {
		if rec.pair.BucketID == bucketID {
			return nil
		}
	}

	rr := newRunningReconciler(ctx, le, c.bus, pair, v)
	c.reconcilers = append(c.reconcilers, rr)
	go func() {
		if err := rr.Execute(); err != nil && err != context.Canceled {
			le.
				WithError(err).
				Warn("reconciler exited with error")
		}
		c.reconcilersMtx.Lock()
		for i, rec := range c.reconcilers {
			if rec.pair.ReconcilerID == reconcilerID {
				c.reconcilers[i] = c.reconcilers[len(c.reconcilers)-1]
				c.reconcilers[len(c.reconcilers)-1] = nil
				c.reconcilers = c.reconcilers[:len(c.reconcilers)-1]
				break
			}
		}
		c.reconcilersMtx.Unlock()
	}()

	return nil
}
