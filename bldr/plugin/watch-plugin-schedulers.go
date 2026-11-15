package bldr_plugin

import (
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/broadcast"
)

// WatchPluginSchedulers calls cb with every plugin host scheduler reachable
// from b and its current status snapshot, then again after each change to the
// scheduler set or a snapshot. snapshots[i] belongs to schedulers[i]. It
// returns when ctx ends, the lookup fails, or cb returns an error.
func WatchPluginSchedulers(
	ctx context.Context,
	b bus.Bus,
	cb func([]PluginScheduler, []*PluginStatusSnapshot) error,
) error {
	// Cancel the scheduler watch when this function returns.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// bcast guards schedulers, collected, and lookupErr.
	var bcast broadcast.Broadcast
	var schedulers []PluginScheduler
	var collected bool
	var lookupErr error
	_, release, err := bus.ExecCollectValuesWatch(
		ctx,
		b,
		NewLookupPluginScheduler(),
		true,
		func(_ []error, vals []LookupPluginSchedulerValue) error {
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				schedulers = slices.Clone(vals)
				collected = true
				broadcast()
			})
			return nil
		},
		func(err error) {
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				lookupErr = err
				broadcast()
			})
		},
	)
	if err != nil {
		return err
	}
	defer release()

	// Deliver each collected scheduler set to the callback until the watch ends.
	for {
		var current []PluginScheduler
		var ready bool
		var setWaitCh <-chan struct{}
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			current, ready, err, setWaitCh = schedulers, collected, lookupErr, getWaitCh()
		})
		if err != nil {
			return err
		}
		if !ready {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-setWaitCh:
				continue
			}
		}

		snapshots := make([]*PluginStatusSnapshot, len(current))
		for i, scheduler := range current {
			snapshots[i] = scheduler.GetPluginStatusCtr().GetValue()
		}
		if err := cb(current, snapshots); err != nil {
			return err
		}

		waitCtx, waitCancel := context.WithCancel(ctx)
		statusCh := waitPluginStatusChange(waitCtx, current, snapshots)
		select {
		case <-ctx.Done():
			waitCancel()
			return ctx.Err()
		case <-setWaitCh:
		case <-statusCh:
		}
		waitCancel()
	}
}

// waitPluginStatusChange returns a channel closed when any scheduler's status
// snapshot differs from the matching entry in snapshots. The watches end with
// ctx.
func waitPluginStatusChange(
	ctx context.Context,
	schedulers []PluginScheduler,
	snapshots []*PluginStatusSnapshot,
) <-chan struct{} {
	changed := make(chan struct{})
	var once sync.Once
	for i, scheduler := range schedulers {
		go func() {
			_, err := scheduler.GetPluginStatusCtr().WaitValueChange(ctx, snapshots[i], nil)
			if err == nil {
				once.Do(func() { close(changed) })
			}
		}()
	}
	return changed
}
