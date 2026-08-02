package seedflight

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

// Seed coordinates concurrent callers around a single fetch invocation.
// The first caller runs the fetch function while waiters block on the supplied
// broadcast and observe the same result; once the fetch completes, a later Run
// call fires fetch again.
//
// Embed Seed next to the broadcast that already guards the related cache state.
type Seed struct {
	inflight bool
	lastErr  error
}

// Run runs fetchFn at most once across concurrent callers using bcast as the
// wait coordination primitive.
func (s *Seed) Run(
	ctx context.Context,
	bcast *broadcast.Broadcast,
	fetchFn func(context.Context) error,
) error {
	return s.RunWhenReady(ctx, bcast, nil, fetchFn)
}

// RunWhenReady runs fetchFn at most once across concurrent callers, skipping
// the fetch when readyFn reports that the guarded cache is already usable.
// readyFn runs while bcast's lock is held.
func (s *Seed) RunWhenReady(
	ctx context.Context,
	bcast *broadcast.Broadcast,
	readyFn func() bool,
	fetchFn func(context.Context) error,
) error {
	for {
		var (
			waitCh <-chan struct{}
			wasIn  bool
			owner  bool
			ready  bool
		)
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			if readyFn != nil && readyFn() {
				ready = true
				return
			}
			if s.inflight {
				wasIn = true
				return
			}
			s.inflight = true
			s.lastErr = nil
			owner = true
		})

		if ready {
			return nil
		}
		if owner {
			err := fetchFn(ctx)
			bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				s.inflight = false
				s.lastErr = err
				broadcast()
			})
			return err
		}

		if !wasIn {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}

		var (
			done bool
			err  error
		)
		bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if !s.inflight {
				done = true
				err = s.lastErr
			}
		})
		if done {
			return err
		}
	}
}
