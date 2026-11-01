package packmove

import (
	"context"
	"sync"
)

// commitOrder admits the batches to the log one at a time in the order the
// reader numbered them, so concurrent uploads record in key order.
type commitOrder struct {
	// mtx guards next and turn.
	mtx sync.Mutex
	// next is the number of the batch whose turn it is.
	next uint64
	// turn is closed and replaced when next advances; nil until a waiter
	// needs it.
	turn chan struct{}
}

// wait blocks until it is batch seq's turn or ctx ends. The caller calls
// advance when its turn is over.
func (o *commitOrder) wait(ctx context.Context, seq uint64) error {
	for {
		o.mtx.Lock()
		if o.next == seq {
			o.mtx.Unlock()
			return nil
		}
		if o.turn == nil {
			o.turn = make(chan struct{})
		}
		turn := o.turn
		o.mtx.Unlock()
		select {
		case <-turn:
		case <-ctx.Done():
			return context.Cause(ctx)
		}
	}
}

// advance ends the current turn and wakes the waiters.
func (o *commitOrder) advance() {
	o.mtx.Lock()
	o.next++
	if o.turn != nil {
		close(o.turn)
		o.turn = nil
	}
	o.mtx.Unlock()
}
