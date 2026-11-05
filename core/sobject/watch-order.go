package sobject

import (
	"context"
)

// WatchOrder calls fn with the agreed order of so's state, then with each
// later state whose checkpoint, order or stable prefix differs, until ctx ends
// or fn returns an error, which WatchOrder returns.
// fn runs on the calling goroutine, one event at a time.
func WatchOrder(ctx context.Context, so SharedObject, fn func(*OrderEvent) error) error {
	// Watch the state.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()

	// Report each state whose order changed.
	var snap SharedObjectStateSnapshot
	var last *OrderEvent
	for {
		snap, err = ctr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}
		if snap == nil {
			continue
		}
		ev, err := ReadOrder(ctx, snap)
		if err != nil {
			return err
		}
		if ev.SameOrder(last) {
			continue
		}
		if err := fn(ev); err != nil {
			return err
		}
		last = ev
	}
}
