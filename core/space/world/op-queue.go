//go:build space_adapter_decl

package space_world

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/core/sobject"
)

// opQueue queues the admitted candidates' operations on the SharedObject one
// at a time, in admission order, so each operation enters the author's chain
// after the one whose World it was built on.
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
func (q *opQueue) push(adm *spaceAdmission) {}

// remove takes adm out of the queue when QueueOperation has not taken it, and
// reports whether it did.
func (q *opQueue) remove(adm *spaceAdmission) bool {
	return false
}

// execute passes each waiting admission's operation to QueueOperation until
// ctx ends, marking it placed with its local ID or failed with the error.
func (q *opQueue) execute(ctx context.Context) error {
	return errAdapterUnbuilt
}
