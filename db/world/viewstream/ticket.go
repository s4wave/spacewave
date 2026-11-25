package viewstream

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// Ticket is one view run's claim on an Admission, taken at the run's first
// cold read rather than before the run starts, so a first view whose blocks
// are warm never queues behind cold ones. It implements block.ColdReadGate:
// install it with block.WithColdReadGate on the run's context, and Close it
// when the run ends. Once taken, the slot stays held to the run's end, so it
// still bounds the CPU a heavy view spends, unless the run lends it back
// while it waits on another reader's work; the run's next cold read takes a
// slot again.
//
// A Ticket is idle, acquiring, held or closed. Concurrent cold reads of one
// run share one acquisition.
type Ticket struct {
	// a is the admission the slot comes from.
	a *Admission
	// mtx guards the fields below.
	mtx sync.Mutex
	// release gives back the held slot; nil unless held.
	release func()
	// acquiring closes when the acquisition in flight ends; nil unless
	// acquiring.
	acquiring chan struct{}
	// closed is set once the run ended; a closed ticket admits at once.
	closed bool
}

// Ticket returns an idle ticket for one view run. A nil admission's ticket
// admits every read.
func (a *Admission) Ticket() *Ticket {
	return &Ticket{a: a}
}

// EnterCold waits until the run holds a slot, taking one if it holds none.
// It fails only when ctx ends first.
func (t *Ticket) EnterCold(ctx context.Context) error {
	for {
		// Return while held or closed, or wait on the acquisition in flight.
		t.mtx.Lock()
		if t.closed || t.release != nil {
			t.mtx.Unlock()
			return nil
		}
		if wait := t.acquiring; wait != nil {
			t.mtx.Unlock()
			select {
			case <-wait:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		// Acquire for the run, outside the lock.
		done := make(chan struct{})
		t.acquiring = done
		t.mtx.Unlock()
		release, err := t.a.Acquire(ctx)

		// Hold the slot, unless the run ended meanwhile.
		t.mtx.Lock()
		t.acquiring = nil
		close(done)
		if err == nil && t.closed {
			t.mtx.Unlock()
			release()
			return nil
		}
		t.release = release
		t.mtx.Unlock()
		return err
	}
}

// Lend gives back the held slot, if any, so the ticket is idle again.
func (t *Ticket) Lend() {
	// Take the slot under the lock and give it back outside it.
	t.mtx.Lock()
	release := t.release
	t.release = nil
	t.mtx.Unlock()
	if release != nil {
		release()
	}
}

// Close ends the run's claim and gives back its slot; later reads pass.
func (t *Ticket) Close() {
	t.mtx.Lock()
	t.closed = true
	t.mtx.Unlock()
	t.Lend()
}

// _ is a type assertion.
var _ block.ColdReadGate = (*Ticket)(nil)
