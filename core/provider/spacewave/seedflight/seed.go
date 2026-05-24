package seedflight

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
)

// Seed coordinates concurrent callers around a single fetch invocation.
// The owner runs the fetch function while waiters block on the supplied
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
