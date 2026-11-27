package space_world

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
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
	// stopped is why the queue passes nothing more to QueueOperation, or
	// nil. Guarded by bcast.
	stopped error
}

// stop ends each admission the queue takes from now on with err, without
// passing it to QueueOperation. The first error stays.
func (q *opQueue) stop(err error) {
	q.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if q.stopped == nil {
			q.stopped = err
			broadcast()
		}
	})
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
// and position, and the error. The driver decides the admission of an
// operation with data once the driver applies the operation.
func (q *opQueue) execute(ctx context.Context, queued func(*spaceAdmission, string, *sobject.SOOperationPosition, error)) error {
	for {
		// Take the oldest waiting admission, waiting for one.
		var adm *spaceAdmission
		var stopped error
		err := q.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
			if len(q.waiting) == 0 {
				return false, nil
			}
			adm, q.waiting, stopped = q.waiting[0], q.waiting[1:], q.stopped
			return true, nil
		})
		if err != nil {
			return context.Canceled
		}

		// End it unsent once the queue stopped.
		if stopped != nil {
			queued(adm, "", nil, stopped)
			continue
		}

		// Queue it with a receipt, except in legacy mode or for an acknowledgment.
		localID, pos, err := q.queueOperation(ctx, adm)
		if err != nil && ctx.Err() != nil {
			return context.Canceled
		}
		queued(adm, localID, pos, err)
	}
}

// queueOperation requires a signer that publishes identity before a WorldOp
// enters replay. An acknowledgment has no WorldOp for replay to attribute.
func (q *opQueue) queueOperation(ctx context.Context, adm *spaceAdmission) (string, *sobject.SOOperationPosition, error) {
	// Keep the original queue path for acknowledgments and legacy mode.
	if adm.op == nil || adm.a.legacyReplayMatcher {
		return q.so.QueueOperation(ctx, adm.op)
	}

	// Reject before queueing if the provider cannot publish early identity.
	so, ok := q.so.(sobject.SignedOperationQueuer)
	if !ok {
		return "", nil, errors.New("shared object does not publish signed operation receipts")
	}
	op, err := adm.a.expectWrite(adm)
	if err != nil {
		return "", nil, err
	}
	return so.QueueOperationWithReceipt(ctx, op, &adm.receipt)
}
