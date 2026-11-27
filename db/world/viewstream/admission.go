package viewstream

import (
	"context"
	"slices"
	"sync"
	"time"
)

// Admission limits how many view streams build their first view at once.
// A restarted backend meets thousands of reconnecting streams in the same
// second, each asking for a full first view on a cold process. Building them
// all at once pins every core for minutes and starves the World writer. The
// application sizes the limit, such as one slot per two cores, so the writer
// and the streams already open always find a core, and SetSlots resizes the
// limit while streams wait. Admission admits in priority order and first come
// first served within a priority.
type Admission struct {
	// observer counts the streams admitted and waiting, and their wait.
	observer AdmissionObserver
	// hold is how long a slot stays held before it returns by itself:
	// selected by the application. Zero keeps the slot until release.
	hold time.Duration
	// mtx guards the fields below.
	mtx sync.Mutex
	// slots is the number of slots, as NewAdmission or SetSlots set it.
	slots int
	// free is the number of slots no first view holds. It is negative after
	// SetSlots lowered slots below the slots held, until enough return.
	free int
	// waiting holds the streams waiting for a slot, oldest first, by
	// priority.
	waiting [][]*waiter
	// defaultPriority applies when a context specifies no priority.
	defaultPriority int
}

// NewAdmission bounds first-read work by slots and orders its priorities.
// hold > 0 returns an abandoned slot after that duration. hold <= 0 keeps the
// slot until release. Slots and priorities have a minimum of one.
func NewAdmission(slots, priorities, defaultPriority int, hold time.Duration, observer AdmissionObserver) *Admission {
	priorities = max(1, priorities)
	slots = max(1, slots)
	return &Admission{
		observer:        observer,
		hold:            hold,
		slots:           slots,
		free:            slots,
		waiting:         make([][]*waiter, priorities),
		defaultPriority: max(0, min(defaultPriority, priorities-1)),
	}
}

// waiter is one stream waiting in line for a slot.
type waiter struct {
	// granted closes when the stream is given a slot.
	granted chan struct{}
	// p is the priority of the line the stream waits in, guarded by the
	// Admission's mtx. raise moves it to a more urgent line.
	p int
}

// Acquire waits for a slot at the priority ctx carries and returns the
// function that gives it back. The caller gives it back when its first read
// returns or its stream ends. The slot also returns by itself after the hold.
// A nil admission admits at once. Acquire fails only when ctx ends first.
func (a *Admission) Acquire(ctx context.Context) (func(), error) {
	if a == nil {
		return func() {}, nil
	}
	return a.acquire(ctx, a.newWaiter(a.priority(ctx)))
}

// priority returns the line ctx asks for, or the default priority when ctx
// carries no priority or a priority out of range.
func (a *Admission) priority(ctx context.Context) int {
	p := Priority(ctx, a.defaultPriority)
	if p < 0 || p >= len(a.waiting) {
		return a.defaultPriority
	}
	return p
}

// newWaiter returns a stream that waits at priority p once it joins a line.
func (a *Admission) newWaiter(p int) *waiter {
	return &waiter{granted: make(chan struct{}), p: p}
}

// acquire waits for a slot as w, at w's priority when it joins the line or
// the one raise moved it to since.
func (a *Admission) acquire(ctx context.Context, w *waiter) (func(), error) {
	// Take a free slot when no stream waits ahead.
	a.mtx.Lock()
	if a.free > 0 {
		a.free--
		p := w.p
		a.mtx.Unlock()
		if a.observer != nil {
			a.observer.Admitted(p, 0)
		}
		return a.held(), nil
	}

	// Otherwise wait in line at the stream's priority.
	a.waiting[w.p] = append(a.waiting[w.p], w)
	p := w.p
	a.mtx.Unlock()
	if a.observer != nil {
		a.observer.Waiting(p, 1)
	}
	start := time.Now()
	select {
	case <-w.granted:
		p = a.line(w)
		if a.observer != nil {
			a.observer.Waiting(p, -1)
			a.observer.Admitted(p, time.Since(start))
		}
		return a.held(), nil
	case <-ctx.Done():
	}

	// Leave the line. A slot granted meanwhile passes to the next stream.
	a.mtx.Lock()
	left := a.leaveLocked(w, w.p)
	p = w.p
	a.mtx.Unlock()
	if a.observer != nil {
		a.observer.Waiting(p, -1)
	}
	if !left {
		a.release()
	}
	return nil, ctx.Err()
}

// line returns the priority of the line w waits or waited in.
func (a *Admission) line(w *waiter) int {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return w.p
}

// raise moves w to the line of priority p when p is more urgent than its
// own, behind the streams already waiting there. A stream not yet in line
// joins at p. One already granted keeps its slot.
func (a *Admission) raise(w *waiter, p int) {
	// Lower the waiter's priority, and move it when it stands in line.
	a.mtx.Lock()
	from := w.p
	if p < 0 || p >= from {
		a.mtx.Unlock()
		return
	}
	w.p = p
	moved := a.leaveLocked(w, from)
	if moved {
		a.waiting[p] = append(a.waiting[p], w)
	}
	a.mtx.Unlock()

	// Count the move.
	if moved && a.observer != nil {
		a.observer.Waiting(from, -1)
		a.observer.Waiting(p, 1)
	}
}

// leaveLocked takes w out of the line of priority p and reports whether it
// stood there. The caller holds mtx.
func (a *Admission) leaveLocked(w *waiter, p int) bool {
	i := slices.Index(a.waiting[p], w)
	if i < 0 {
		return false
	}
	a.waiting[p] = slices.Delete(a.waiting[p], i, i+1)
	return true
}

// held returns the function that gives back one slot, once, and gives it back
// by itself after the hold, counting the expiry.
func (a *Admission) held() func() {
	var once sync.Once
	if a.hold <= 0 {
		return func() { once.Do(a.release) }
	}
	timer := time.AfterFunc(a.hold, func() {
		once.Do(func() {
			if a.observer != nil {
				a.observer.Expired()
			}
			a.release()
		})
	})
	return func() {
		timer.Stop()
		once.Do(a.release)
	}
}

// SetSlots changes the number of slots to slots, at least one, while streams
// hold and wait for them. Raising it admits as many waiting streams at once,
// in priority order. Lowering the count takes the free slots first and retires
// each held slot past the new count as it returns, so the streams already
// admitted finish undisturbed and no new one starts until the held slots fit.
func (a *Admission) SetSlots(slots int) {
	// Move free by the change in count. free goes negative while held slots
	// past a lowered count are still out, and release retires those.
	slots = max(1, slots)
	a.mtx.Lock()
	defer a.mtx.Unlock()
	a.free += slots - a.slots
	a.slots = slots

	// Admit waiting streams into any slots the raise freed.
	for a.free > 0 && a.grantLocked() {
		a.free--
	}
}

// Slots returns the number of slots.
func (a *Admission) Slots() int {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	return a.slots
}

// release grants a returned slot to the first waiting stream of the highest
// priority, or frees the slot. A slot past the count SetSlots lowered retires.
func (a *Admission) release() {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if a.free >= 0 && a.grantLocked() {
		return
	}
	a.free++
}

// grantLocked hands a slot to the first waiting stream of the highest
// priority, and reports whether one waited. The caller holds mtx.
func (a *Admission) grantLocked() bool {
	for p := range a.waiting {
		if len(a.waiting[p]) != 0 {
			close(a.waiting[p][0].granted)
			a.waiting[p] = a.waiting[p][1:]
			return true
		}
	}
	return false
}
