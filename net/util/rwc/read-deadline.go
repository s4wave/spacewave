package rwc

import (
	"context"
	"os"
	"time"

	"github.com/aperturerobotics/util/broadcast"
)

// readDeadline guards a connection's deadline and wakes reads when it changes.
// The deadline and its change subscription are always read under one lock.
type readDeadline struct {
	// bcast guards deadline and notifies pending reads.
	bcast broadcast.Broadcast
	// deadline is zero when reads have no time limit.
	deadline time.Time
}

// set publishes the deadline and wakes every pending read.
func (d *readDeadline) set(t time.Time) {
	// Publish the new deadline before waking its readers.
	locked := d.bcast.Lock()
	defer locked.Unlock()
	if d.deadline.Equal(t) {
		return
	}
	d.deadline = t
	locked.Broadcast()
}

// read waits for a packet, cancellation, or the current read deadline.
func (d *readDeadline) read(ctx context.Context, packets <-chan []byte) ([]byte, bool, error) {
	// Reuse one timer across deadline changes and stop it on every return.
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	// Recheck the guarded deadline after every notification or timer expiry.
	for {
		// Snapshot the deadline and its wakeup under the same lock.
		locked := d.bcast.Lock()
		deadline := d.deadline
		changed := locked.WaitCh()
		locked.Unlock()

		// Arm a timer only for a nonzero, unexpired deadline.
		var expired <-chan time.Time
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				return nil, false, os.ErrDeadlineExceeded
			}
			if timer == nil {
				timer = time.NewTimer(remaining)
			} else {
				timer.Reset(remaining)
			}
			expired = timer.C
		}

		// Wait without the lock so setters can replace or clear the deadline.
		select {
		case <-ctx.Done():
			return nil, false, ctx.Err()
		case packet, ok := <-packets:
			return packet, ok, nil
		case <-changed:
		case <-expired:
		}
	}
}
