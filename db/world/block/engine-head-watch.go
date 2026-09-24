package world_block

import (
	"context"
	"errors"
)

// watchCoordinatorHead reconciles durable head changes once for all Engine
// readers. Close cancels the context and joins headWatchDone before releasing
// the stores used by the refresh callback.
func (e *Engine) watchCoordinatorHead(ctx context.Context) {
	defer close(e.headWatchDone)
	if err := e.runCoordinatorHeadWatch(ctx); err != nil && ctx.Err() == nil {
		e.publishHeadWatchError(err)
	}
}

// runCoordinatorHeadWatch subscribes before its first refresh so publication
// racing subscription setup is discovered through either the read or an event.
func (e *Engine) runCoordinatorHeadWatch(ctx context.Context) error {
	// Baseline durable generation before attaching the event stream.
	snapshot, err := e.writeCoordinator.Snapshot(ctx, e.writeCoordScope)
	if err != nil {
		return err
	}
	watch, err := e.writeCoordinator.Watch(ctx, e.writeCoordScope, snapshot.Generation)
	if err != nil {
		return err
	}
	defer watch.Close()

	// Reconcile once after setup and once for each advisory event.
	for {
		// Events are advisory. The persisted head remains authoritative, including
		// after a writer releases its lease without publishing an event payload.
		head, err := e.writeHeadRefresh(ctx)
		if err == nil && head != nil && !head.GetRootRef().GetEmpty() {
			err = e.AdoptRootRefFromWatch(ctx, head)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		e.publishHeadWatchError(err)

		// Keep observation active until engine shutdown or subscription failure.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case _, ok := <-watch.Events():
			if !ok {
				return errors.New("world write coordinator watch closed")
			}
		}
	}
}

// publishHeadWatchError wakes revision waiters on failure and clears the error
// after a later successful refresh. Head adoption owns publication wakeups.
func (e *Engine) publishHeadWatchError(err error) {
	// Publish failure under the same lock used by revision waiters.
	locked := e.bcast.Lock()
	if !e.closed {
		e.headWatchErr = err
		if err != nil {
			e.signalHeadLocked()
		}
	}
	locked.Unlock()

	// Report observation failures without holding publication authority.
	if err != nil {
		e.le.WithError(err).Warn("world coordinator head watch failed")
	}
}
