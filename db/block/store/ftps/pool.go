package block_store_ftps

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// interactiveReserve is how many slots refill and background calls leave for
// interactive calls, so a request never waits behind a pool full of them.
const interactiveReserve = 2

// slotPool hands out the client's slots by read class.
//
// An interactive call takes a free slot at once, or the next one released
// before any other class. Refill and background calls share the rest: they
// hold at most capped slots at once, leaving interactiveReserve for
// interactive calls, and take a slot only while no interactive call waits.
// A waiting call whose context raises it (block.WithRaise) waits on as
// interactive. Each class waits in arrival order.
//
// Invariant: a free slot exists only while no waiter could take it.
type slotPool struct {
	// mu guards the fields below.
	mu sync.Mutex
	// free holds the slots no call holds.
	free []*slot
	// n is the number of slots.
	n int
	// capped is the most slots refill and background calls hold at once.
	capped int
	// cappedHeld counts the slots refill and background calls hold.
	cappedHeld int
	// interactive and other queue the waiting calls of each kind.
	interactive, other []*slotWaiter
}

// slotWaiter is one call waiting for a slot.
type slotWaiter struct {
	// grant receives the slot. It holds one, so a grant never blocks.
	grant chan *slot
	// capped is whether the call counts against capped when granted.
	capped bool
}

// newSlotPool builds a pool of slots. With fewer than interactiveReserve+1
// slots, refill and background calls still hold one.
func newSlotPool(slots []*slot) *slotPool {
	n := len(slots)
	return &slotPool{free: slots, n: n, capped: max(n-interactiveReserve, 1)}
}

// take waits for a slot for a call in the read class of ctx. The caller
// returns the slot with put. It fails only when ctx ends first.
func (p *slotPool) take(ctx context.Context) (*slot, error) {
	// Take a free slot the call's class may hold.
	capped := block.ReadClassOf(ctx) != block.ReadInteractive
	p.mu.Lock()
	if s := p.takeFreeLocked(capped); s != nil {
		p.mu.Unlock()
		return s, nil
	}

	// Queue for the next slot released to this class.
	w := &slotWaiter{grant: make(chan *slot, 1), capped: capped}
	if capped {
		p.other = append(p.other, w)
	} else {
		p.interactive = append(p.interactive, w)
	}
	p.mu.Unlock()

	// Wait for the grant, moving to the interactive queue when raised, or
	// leave the queue when ctx ends.
	var raised <-chan struct{}
	if capped {
		raised = block.Raised(ctx)
	}
	for {
		select {
		case s := <-w.grant:
			return s, nil
		case <-raised:
			raised = nil
			p.raise(w)
		case <-ctx.Done():
			p.leave(w)
			return nil, context.Cause(ctx)
		}
	}
}

// takeFreeLocked takes a free slot for a call of the kind given, or returns
// nil when it must wait. By the invariant, a free slot means no waiter of a
// kind that may take it.
func (p *slotPool) takeFreeLocked(capped bool) *slot {
	// Wait when no slot is free or the call's kind holds its cap.
	if len(p.free) == 0 || (capped && p.cappedHeld >= p.capped) {
		return nil
	}

	// Take the last free slot and count it against its kind.
	s := p.free[len(p.free)-1]
	p.free = p.free[:len(p.free)-1]
	s.capped = capped
	if capped {
		p.cappedHeld++
	}
	return s
}

// put returns a slot taken with take and hands it to the next waiter.
func (p *slotPool) put(s *slot) {
	// Free the slot and grant it to the next eligible waiter.
	p.mu.Lock()
	if s.capped {
		p.cappedHeld--
	}
	p.free = append(p.free, s)
	p.grantLocked()
	p.mu.Unlock()
}

// grantLocked hands free slots to waiters, interactive first, until no free
// slot or no eligible waiter remains.
func (p *slotPool) grantLocked() {
	for len(p.free) != 0 {
		var w *slotWaiter
		switch {
		case len(p.interactive) != 0:
			w, p.interactive = p.interactive[0], p.interactive[1:]
		case len(p.other) != 0 && p.cappedHeld < p.capped:
			w, p.other = p.other[0], p.other[1:]
		default:
			return
		}
		w.grant <- p.takeFreeLocked(w.capped)
	}
}

// raise moves a waiting refill or background call to the interactive queue,
// unless it was already granted a slot.
func (p *slotPool) raise(w *slotWaiter) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i := indexWaiter(p.other, w); i >= 0 {
		p.other = append(p.other[:i], p.other[i+1:]...)
		w.capped = false
		p.interactive = append(p.interactive, w)
		p.grantLocked()
	}
}

// leave removes a call whose context ended from its queue. A slot granted to
// it meanwhile goes back to the pool.
func (p *slotPool) leave(w *slotWaiter) {
	// Remove the waiter from whichever queue still holds it.
	p.mu.Lock()
	if i := indexWaiter(p.interactive, w); i >= 0 {
		p.interactive = append(p.interactive[:i], p.interactive[i+1:]...)
		p.mu.Unlock()
		return
	}
	if i := indexWaiter(p.other, w); i >= 0 {
		p.other = append(p.other[:i], p.other[i+1:]...)
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()

	// No queue holds it, so it was granted a slot: return the slot.
	p.put(<-w.grant)
}

// indexWaiter returns the position of w in queue, or -1.
func indexWaiter(queue []*slotWaiter, w *slotWaiter) int {
	for i, q := range queue {
		if q == w {
			return i
		}
	}
	return -1
}
