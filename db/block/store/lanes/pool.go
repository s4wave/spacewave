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
// at once, or the next lane released before any other class. An interactive
// call comes next. Refill and background calls share the rest: they hold at
// most capped lanes at once, leaving reserve lanes for the urgent and
// interactive calls, and take a lane only while none of those waits, refill
// before background. SetCappedFloor lets refill and background calls hold a
// few lanes ahead of the other classes, so they keep progressing under any
// foreground load. The pool's limit, which SetLimit moves, bounds the lanes
// all calls hold at once, and the reserve comes out of the limit. While the
// limit is zero, SetProbes lets a few urgent calls through as probes. A
// waiting call whose class rises (block.WithRaise, block.WithDemand) moves to
// the queue of its new class. Each class waits in arrival order, except that
// a released lane goes to a waiter among the first few of its class that
// prefers it.
//
// Invariant: a free lane exists only while no waiter could take it.
type Pool[T comparable] struct {
	// mu guards the fields below.
	mu sync.Mutex
	// free holds the lanes no call holds.
	free []T
	// n is the number of lanes.
	n int
	// reserve is how many lanes of the limit refill and background calls
	// leave for urgent and interactive ones.
	reserve int
	// limit is the most lanes all calls hold at once, from zero, which
	// pauses the pool, to n.
	limit int
	// held counts the lanes calls hold.
	held int
	// cappedHeld counts the lanes refill and background calls hold.
	cappedHeld int
	// cappedFloor is how many lanes refill and background calls may hold
	// ahead of urgent and interactive waiters.
	cappedFloor int
	// probes is how many urgent calls may hold a lane while the limit is
	// zero.
	probes int
	// probeHeld counts the lanes probes hold.
	probeHeld int
	// floorLast is the capped kind that took the last lane granted below
	// the capped floor. The next such grant goes to the other kind when
	// that kind waits.
	floorLast kind
	// queues holds the waiting calls of each kind, most urgent first.
	queues [kinds][]*waiter[T]
}

// kind is the queue a call waits in.
type kind int

const (
	kindUrgent kind = iota
	kindInteractive
	kindRefill
	kindBackground
	kinds
)

// capped reports whether calls of k count against the refill and background
// cap.
func (k kind) capped() bool {
	return k >= kindRefill
}

// kindOf returns the queue of a call of class.
func kindOf(class block.ReadClass) kind {
	switch {
	case class == block.ReadUrgent:
		return kindUrgent
	case class.Foreground():
		return kindInteractive
	case class == block.ReadRefill:
		return kindRefill
	default:
		return kindBackground
	}
}

// Lease is a lane a call holds until it returns it with Pool.Put.
type Lease[T comparable] struct {
	// Lane is the lane the call holds.
	Lane T
	// Capped reports whether the lease counts against the refill and
	// background cap: the call was neither urgent nor interactive.
	Capped bool
	// Probe reports whether the call took the lane as a probe while the
	// limit was zero.
	Probe bool
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
	return &Pool[T]{free: slices.Clone(lanes), n: n, reserve: reserve, limit: n}
}

// SetLimit bounds the lanes all calls hold at once to n, clamped to zero and
// the number of lanes. Zero pauses the pool: calls holding lanes keep them,
// and every new call waits until a later SetLimit raises the limit. A lowered
// limit takes effect as held lanes return.
func (p *Pool[T]) SetLimit(n int) {
	p.mu.Lock()
	p.limit = min(max(n, 0), p.n)
	p.grantLocked()
	p.mu.Unlock()
}

// SetCappedFloor lets refill and background calls hold up to n lanes ahead
// of waiting urgent and interactive calls, within the limit, so a foreground
// load above the limit cannot starve them. The default floor is zero.
func (p *Pool[T]) SetCappedFloor(n int) {
	p.mu.Lock()
	p.cappedFloor = max(n, 0)
	p.grantLocked()
	p.mu.Unlock()
}

// SetProbes lets up to n urgent calls hold a lane each while the limit is
// zero, as probes of whether the link has recovered. The default probe count
// is zero.
func (p *Pool[T]) SetProbes(n int) {
	p.mu.Lock()
	p.probes = max(n, 0)
	p.grantLocked()
	p.mu.Unlock()
}

// cappedLocked returns the most lanes refill and background calls may hold
// at once: the limit less the reserve, and at least the floor and one.
func (p *Pool[T]) cappedLocked() int {
	return max(p.limit-p.reserve, p.cappedFloor, 1)
}

// admitLocked reports whether a call of kind k may take a free lane now, and
// whether it takes it as a probe.
func (p *Pool[T]) admitLocked(k kind) (ok, probe bool) {
	switch {
	case len(p.free) == 0:
		return false, false
	case p.limit == 0:
		return k == kindUrgent && p.probeHeld < p.probes, true
	case p.held >= p.limit:
		return false, false
	case k.capped() && p.cappedHeld >= p.cappedLocked():
		return false, false
	}
	return true, false
}

// Len returns the number of lanes.
func (p *Pool[T]) Len() int {
	return p.n
}

// Take waits for a lane for a call in the read class of ctx. prefer, when
// not nil, picks the lanes the call would rather take, such as the one that
// already has the object open. prefer runs under the pool's lock, so it must
// not block. The caller returns the lease with Put. Take fails only when ctx
// ends first.
func (p *Pool[T]) Take(ctx context.Context, prefer func(T) bool) (*Lease[T], error) {
	// Take a free lane the call's class may hold. Read the raise channel
	// before the class, so a rise between the two still wakes the wait.
	raised := block.Raised(ctx)
	w := &waiter[T]{grant: make(chan *Lease[T], 1), kind: kindOf(block.ReadClassOf(ctx)), prefer: prefer}
	p.mu.Lock()
	if l := p.takeFreeLocked(w); l != nil {
		p.mu.Unlock()
		return l, nil
	}

	// Queue for the next lane released to this kind.
	p.queues[w.kind] = append(p.queues[w.kind], w)
	p.mu.Unlock()

	// Wait for the grant, moving to the queue of a risen class, or leave
	// the queue when ctx ends.
	for {
		select {
		case l := <-w.grant:
			return l, nil
		case <-raised:
			raised = p.raise(ctx, w)
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
	// Wait when no lane is free, the calls hold the limit, or the call's
	// kind holds its cap.
	ok, probe := p.admitLocked(w.kind)
	if !ok {
		return nil
	}

	// Take a preferred free lane, else the last one.
	i := len(p.free) - 1
	if w.prefer != nil {
		if j := slices.IndexFunc(p.free, w.prefer); j >= 0 {
			i = j
		}
	}
	lane := p.free[i]
	p.free = slices.Delete(p.free, i, i+1)

	// Count the lane against the limit, its kind and the probes.
	p.held++
	capped := w.kind.capped()
	if capped {
		p.cappedHeld++
	}
	if probe {
		p.probeHeld++
	}
	return &Lease[T]{Lane: lane, Capped: capped, Probe: probe}
}

// Put returns a lease taken with Take and hands its lane to the next waiter.
func (p *Pool[T]) Put(l *Lease[T]) {
	// Free the lane and grant it to the next eligible waiter.
	p.mu.Lock()
	p.held--
	if l.Capped {
		p.cappedHeld--
	}
	if l.Probe {
		p.probeHeld--
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

// nextKindLocked returns the kind whose waiter takes the next lane, or -1
// when no waiter may take one now: refill and background below their floor
// first, alternating between them so neither starves the other, then the
// most urgent kind.
func (p *Pool[T]) nextKindLocked() kind {
	if p.cappedHeld < p.cappedFloor {
		first, second := kindRefill, kindBackground
		if p.floorLast == kindRefill {
			first, second = second, first
		}
		for _, k := range [2]kind{first, second} {
			if ok, _ := p.admitLocked(k); ok && len(p.queues[k]) != 0 {
				p.floorLast = k
				return k
			}
		}
	}
	for k := range kinds {
		if ok, _ := p.admitLocked(k); ok && len(p.queues[k]) != 0 {
			return k
		}
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

// raise moves a waiting call whose class rose to the queue of its new class,
// unless it was already granted a lane, and returns the channel of its next
// rise.
func (p *Pool[T]) raise(ctx context.Context, w *waiter[T]) <-chan struct{} {
	// Read the next rise's channel before the class, as Take does.
	raised := block.Raised(ctx)
	k := kindOf(block.ReadClassOf(ctx))
	p.mu.Lock()
	defer p.mu.Unlock()
	if k >= w.kind {
		return raised
	}
	if i := slices.Index(p.queues[w.kind], w); i >= 0 {
		p.queues[w.kind] = slices.Delete(p.queues[w.kind], i, i+1)
		w.kind = k
		p.queues[k] = append(p.queues[k], w)
		p.grantLocked()
	}
	return raised
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
	return p.cappedHeld, len(p.queues[kindRefill]) + len(p.queues[kindBackground])
}

// Stats returns a snapshot of the pool, for status and metrics.
func (p *Pool[T]) Stats() Stats {
	p.mu.Lock()
	defer p.mu.Unlock()
	return Stats{
		Limit:       p.limit,
		Held:        p.held,
		Urgent:      len(p.queues[kindUrgent]),
		Interactive: len(p.queues[kindInteractive]),
		Refill:      len(p.queues[kindRefill]),
		Background:  len(p.queues[kindBackground]),
	}
}
