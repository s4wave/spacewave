package volume_controller

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	"github.com/s4wave/spacewave/db/mqueue"
	volume "github.com/s4wave/spacewave/db/volume"
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
	filledQueues, err := v.ListFilledReconcilerEventQueues(ctx)
	if err != nil {
		return err
	}

	c.syncReconcilerKeys(filledQueues, true)
	return nil
}

// bucketLogger returns the logger for a bucket id
func bucketLogger(le *logrus.Entry, id string) *logrus.Entry {
	return le.WithField("bucket-id", id)
}

// wakeReconcilerQueue attempts to start the process of waking a reconciler.
func (c *Controller) wakeReconcilerQueue(
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

	eq, err := v.GetReconcilerEventQueue(ctx, pair)
	if err != nil {
		return nil, errors.Wrap(err, "get reconciler event queue")
	}

	if len(event) != 0 {
		_, err := eq.Push(ctx, event)
		if err != nil {
			return nil, err
		}
	}

	c.syncReconcilerKeys([]bucket_store.BucketReconcilerPair{pair}, false)
	return eq, nil
}

// syncReconcilerKeys merges or replaces the desired reconciler key set, then
// synchronizes the keyed reconciler lifecycle to that set.
func (c *Controller) syncReconcilerKeys(
	keys []bucket_store.BucketReconcilerPair,
	replace bool,
) {
	c.reconcilerMtx.Lock()
	if replace {
		c.reconcilerKeys = make(map[bucket_store.BucketReconcilerPair]struct{}, len(keys))
	}
	for _, key := range keys {
		c.reconcilerKeys[key] = struct{}{}
	}
	syncKeys := make([]bucket_store.BucketReconcilerPair, 0, len(c.reconcilerKeys))
	for key := range c.reconcilerKeys {
		syncKeys = append(syncKeys, key)
	}
	c.reconcilerMtx.Unlock()

	c.reconcilers.SyncKeys(syncKeys, false)
	for _, key := range keys {
		if rr, ok := c.reconcilers.GetKey(key); ok && !rr.IsRunning() {
			_, _ = c.reconcilers.ResetRoutine(key)
		}
	}
}

// removeReconcilerKey removes a reconciler key from the desired set and the keyed runtime.
func (c *Controller) removeReconcilerKey(key bucket_store.BucketReconcilerPair) {
	c.reconcilerMtx.Lock()
	delete(c.reconcilerKeys, key)
	c.reconcilerMtx.Unlock()
	c.reconcilers.RemoveKey(key)
}

// pushEventToReconcilers pushes an event to all running reconcilers.
// Wakes reconcilers.
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
		_, err = c.wakeReconcilerQueue(ctx, vol, pair, ed)
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
