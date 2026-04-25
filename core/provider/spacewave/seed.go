package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

// providerSeed coordinates concurrent callers around a single fetch
// invocation. The owner runs the fetch function while waiters block on
// the supplied broadcast and observe the same result; once the fetch
// completes a subsequent Run will fire fetch again (singleflight, not
// permanent memoization).
//
// The inflight and lastErr fields are guarded by the broadcast passed
// to Run and must only be touched while holding it. Embed providerSeed
// next to the broadcast that already guards the related cache state so
// all wakes share the same wait channel.
type providerSeed struct {
	// inflight is true while a fetch is running.
	inflight bool
	// lastErr is the result of the most recent owner-run fetch and is
	// returned to waiters that block on the broadcast wait channel.
	lastErr error
}

// Run runs fetchFn at most once across concurrent callers using bcast as
// the wait coordination primitive. The owner runs fetchFn, publishes the
// result to lastErr, and broadcasts; concurrent waiters block on the
// broadcast wait channel and observe the same lastErr. fetchFn is
// responsible for publishing any fetched value into the caller's cache
// (under the same broadcast lock) before returning; Run only
// synchronizes ownership.
func (s *providerSeed) Run(
	ctx context.Context,
	bcast *broadcast.Broadcast,
	fetchFn func(context.Context) error,
) error {
	for {
		var (
			waitCh <-chan struct{}
			wasIn  bool
			owner  bool
		)
		bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			if s.inflight {
				wasIn = true
				return
			}
			s.inflight = true
			s.lastErr = nil
			owner = true
		})

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
