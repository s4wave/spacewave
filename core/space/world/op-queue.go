package space_world

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/sobject"
)

// opQueue queues the admitted operations on the SharedObject one at a time,
// in admission order, so each operation enters the author's chain after the
// one whose World it was built on.
type opQueue struct {
	// so is the SharedObject.
	so sobject.SharedObject

	// bcast guards waiting and wakes the queue goroutine.
	bcast broadcast.Broadcast
	// waiting are the admissions not yet passed to QueueOperation, in
	// admission order. Guarded by bcast.
	waiting []*spaceAdmission
}

// push appends adm to the queue.
func (q *opQueue) push(adm *spaceAdmission) {
	q.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		q.waiting = append(q.waiting, adm)
		broadcast()
	})
}

// drain takes every waiting admission out of the queue and returns them.
func (q *opQueue) drain() []*spaceAdmission {
	var waiting []*spaceAdmission
	q.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		waiting, q.waiting = q.waiting, nil
	})
	return waiting
}

// remove takes adm out of the queue when QueueOperation has not taken it, and
// reports whether it did.
func (q *opQueue) remove(adm *spaceAdmission) bool {
	var removed bool
	q.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if i := slices.Index(q.waiting, adm); i >= 0 {
			q.waiting = slices.Delete(q.waiting, i, i+1)
			removed = true
		}
	})
	return removed
}

// execute passes each waiting admission's operation to QueueOperation until
// ctx ends, then calls queued with the admission, the operation's local ID
// and the error. The driver decides the admission of an operation with data
// once it applies it.
func (q *opQueue) execute(ctx context.Context, queued func(*spaceAdmission, string, error)) error {
	for {
		// Take the oldest waiting admission, or wait for one.
		var adm *spaceAdmission
		var waitCh <-chan struct{}
		q.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if len(q.waiting) != 0 {
				adm = q.waiting[0]
				q.waiting = q.waiting[1:]
			}
			waitCh = getWaitCh()
		})
		if adm == nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-waitCh:
			}
			continue
		}

		// Queue it.
		localID, err := q.so.QueueOperation(ctx, adm.op)
		if err != nil && ctx.Err() != nil {
			return context.Canceled
		}
		queued(adm, localID, err)
	}
}
