// Package lanes admits calls to a bounded link, such as a few sessions to a
// storage server, in the order of their read class.
package lanes

import (
	"context"
	"slices"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// preferWindow is how many waiters of a class a released lane looks through
// for one that prefers it, ahead of the oldest. It bounds how far a waiter
// with a preference may pass older ones of its class.
const preferWindow = 8

// Pool hands out lanes by the read class of each call's context.
//
// A lane is one call's share of the link: an FTPS session, or one of the
// requests an SFTP session keeps in flight. An urgent call takes a free lane
// at once, or the next lane released before any other class; an interactive
// call comes next. Refill and background calls share the rest: they hold at
// most capped lanes at once, leaving reserve lanes for the urgent and
// interactive calls, and take a lane only while none of those waits. A
// waiting call whose context raises it (block.WithRaise) waits on as
// interactive. Each class waits in arrival order, except that a released
// lane goes to a waiter among the first few of its class that prefers it.
//
// Invariant: a free lane exists only while no waiter could take it.
type Pool[T comparable] struct {
	// mu guards the fields below.
	mu sync.Mutex
	// free holds the lanes no call holds.
	free []T
	// n is the number of lanes.
	n int
	// capped is the most lanes refill and background calls hold at once.
	capped int
	// cappedHeld counts the lanes refill and background calls hold.
	cappedHeld int
	// queues holds the waiting calls of each kind, most urgent first.
	queues [kinds][]*waiter[T]
}

// kind is the queue a call waits in.
type kind int

const (
	kindUrgent kind = iota
	kindInteractive
	kindCapped
	kinds
)

// kindOf returns the queue of a call of class.
func kindOf(class block.ReadClass) kind {
	switch {
	case class == block.ReadUrgent:
		return kindUrgent
	case class.Foreground():
		return kindInteractive
	default:
		return kindCapped
	}
}

// Lease is a lane a call holds until it returns it with Pool.Put.
type Lease[T comparable] struct {
	// Lane is the lane the call holds.
	Lane T
	// Capped reports whether the lease counts against the refill and
	// background cap: the call was neither urgent nor interactive.
	Capped bool
}

// waiter is one call waiting for a lane.
type waiter[T comparable] struct {
	// grant receives the lease. It holds one, so a grant never blocks.
	grant chan *Lease[T]
	// kind is the queue the call waits in.
	kind kind
	// prefer reports whether the call would rather take a lane, or is nil.
	prefer func(T) bool
}

// New builds a pool of lanes. The same lane may appear more than once, as
// one per call it serves at a time. Refill and background calls may hold all
// but reserve lanes, and always at least one.
func New[T comparable](lanes []T, reserve int) *Pool[T] {
	n := len(lanes)
	return &Pool[T]{free: slices.Clone(lanes), n: n, capped: max(n-reserve, 1)}
}

// Len returns the number of lanes.
func (p *Pool[T]) Len() int {
	return p.n
}

// Take waits for a lane for a call in the read class of ctx. prefer, when
// not nil, picks the lanes the call would rather take, such as the one that
// already has the object open; it runs under the pool's lock, so it must not
// block. The caller returns the lease with Put. Take fails only when ctx ends
// first.
func (p *Pool[T]) Take(ctx context.Context, prefer func(T) bool) (*Lease[T], error) {
	// Take a free lane the call's class may hold.
	w := &waiter[T]{grant: make(chan *Lease[T], 1), kind: kindOf(block.ReadClassOf(ctx)), prefer: prefer}
	p.mu.Lock()
	if l := p.takeFreeLocked(w); l != nil {
		p.mu.Unlock()
		return l, nil
	}

	// Queue for the next lane released to this kind.
	p.queues[w.kind] = append(p.queues[w.kind], w)
	p.mu.Unlock()

	// Wait for the grant, moving to the interactive queue when raised, or
	// leave the queue when ctx ends.
	var raised <-chan struct{}
	if w.kind == kindCapped {
		raised = block.Raised(ctx)
	}
	for {
		select {
		case l := <-w.grant:
			return l, nil
		case <-raised:
			raised = nil
			p.raise(w)
		case <-ctx.Done():
			p.leave(w)
			return nil, context.Cause(ctx)
		}
	}
}

// takeFreeLocked takes a free lane for w, one it prefers when there is one,
// or returns nil when it must wait. By the invariant, a free lane means no
// waiter of a kind that may take it.
func (p *Pool[T]) takeFreeLocked(w *waiter[T]) *Lease[T] {
	// Wait when no lane is free or the call's kind holds its cap.
	capped := w.kind == kindCapped
	if len(p.free) == 0 || (capped && p.cappedHeld >= p.capped) {
		return nil
	}

	// Take a preferred free lane, else the last one, and count it against
	// its kind.
	i := len(p.free) - 1
	if w.prefer != nil {
		if j := slices.IndexFunc(p.free, w.prefer); j >= 0 {
			i = j
		}
	}
	lane := p.free[i]
	p.free = slices.Delete(p.free, i, i+1)
	if capped {
		p.cappedHeld++
	}
	return &Lease[T]{Lane: lane, Capped: capped}
}

// Put returns a lease taken with Take and hands its lane to the next waiter.
func (p *Pool[T]) Put(l *Lease[T]) {
	// Free the lane and grant it to the next eligible waiter.
	p.mu.Lock()
	if l.Capped {
		p.cappedHeld--
	}
	p.free = append(p.free, l.Lane)
	p.grantLocked()
	p.mu.Unlock()
}

// grantLocked hands free lanes to waiters, most urgent kind first, until no
// free lane or no eligible waiter remains.
func (p *Pool[T]) grantLocked() {
	for len(p.free) != 0 {
		k := p.nextKindLocked()
		if k < 0 {
			return
		}
		queue := p.queues[k]
		i := p.preferredLocked(queue)
		w := queue[i]
		p.queues[k] = slices.Delete(queue, i, i+1)
		w.grant <- p.takeFreeLocked(w)
	}
}

// nextKindLocked returns the most urgent kind with a waiter that may take a
// lane now, or -1.
func (p *Pool[T]) nextKindLocked() kind {
	for k := range kinds {
		if len(p.queues[k]) == 0 {
			continue
		}
		if k == kindCapped && p.cappedHeld >= p.capped {
			continue
		}
		return k
	}
	return -1
}

// preferredLocked returns the position in queue of the first waiter within
// preferWindow that prefers a free lane, or 0 for the oldest.
func (p *Pool[T]) preferredLocked(queue []*waiter[T]) int {
	for i, w := range queue[:min(len(queue), preferWindow)] {
		if w.prefer != nil && slices.ContainsFunc(p.free, w.prefer) {
			return i
		}
	}
	return 0
}

// raise moves a waiting refill or background call to the interactive queue,
// unless it was already granted a lane.
func (p *Pool[T]) raise(w *waiter[T]) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i := slices.Index(p.queues[kindCapped], w); i >= 0 {
		p.queues[kindCapped] = slices.Delete(p.queues[kindCapped], i, i+1)
		w.kind = kindInteractive
		p.queues[kindInteractive] = append(p.queues[kindInteractive], w)
		p.grantLocked()
	}
}

// leave removes a call whose context ended from its queue. A lane granted to
// it meanwhile goes back to the pool.
func (p *Pool[T]) leave(w *waiter[T]) {
	// Remove the waiter from whichever queue still holds it.
	p.mu.Lock()
	for k, queue := range p.queues {
		if i := slices.Index(queue, w); i >= 0 {
			p.queues[k] = slices.Delete(queue, i, i+1)
			p.mu.Unlock()
			return
		}
	}
	p.mu.Unlock()

	// No queue holds it, so it was granted a lane: return the lane.
	p.Put(<-w.grant)
}

// Held returns the lanes refill and background calls hold and the number of
// them waiting, for tests and status.
func (p *Pool[T]) Held() (cappedHeld, cappedWaiting int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cappedHeld, len(p.queues[kindCapped])
}
