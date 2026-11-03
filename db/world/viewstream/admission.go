package viewstream

import (
	"context"
	"sync"
	"time"
)

// Admission limits how many view streams build their first view at once.
// A restarted backend meets thousands of reconnecting streams in the same
// second, each asking for a full first view on a cold process; building them
// all at once pins every core for minutes and starves the World writer. The
// limit admits them one per two cores, so the writer and the streams already
// open always find a core, in priority order and first come first served
// within a priority.
type Admission struct {
	// observer counts the streams admitted and waiting, and their wait.
	observer AdmissionObserver
	// hold is how long a slot stays held before it returns by itself:
	// selected by the application; zero keeps it until release.
	hold time.Duration
	// mtx guards the fields below.
	mtx sync.Mutex
	// free is the number of slots no first view holds.
	free int
	// waiting holds the streams waiting for a slot, oldest first, by
	// priority.
	waiting [][]chan struct{}
	// defaultPriority applies when a context names no priority.
	defaultPriority int
}

// NewAdmission bounds first-read work by slots and orders its priorities.
// hold > 0 returns an abandoned slot after that duration; hold <= 0 keeps it
// until release. Slots and priorities have a minimum of one.
func NewAdmission(slots, priorities, defaultPriority int, hold time.Duration, observer AdmissionObserver) *Admission {
	priorities = max(1, priorities)
	return &Admission{observer: observer, hold: hold, free: max(1, slots), waiting: make([][]chan struct{}, priorities), defaultPriority: max(0, min(defaultPriority, priorities-1))}
}

// Acquire waits for a slot at the priority ctx carries and returns the
// function that gives it back. The caller gives it back when its first read
// returns or its stream ends; the slot also returns by itself after the hold.
// A nil admission admits at once. It fails only when ctx ends first.
func (a *Admission) Acquire(ctx context.Context) (func(), error) {
	// A source without an admission admits every stream.
	if a == nil {
		return func() {}, nil
	}

	// Take a free slot when no stream waits ahead.
	p := Priority(ctx, a.defaultPriority)
	if p < 0 || p >= len(a.waiting) {
		p = a.defaultPriority
	}
	a.mtx.Lock()
	if a.free > 0 {
		a.free--
		a.mtx.Unlock()
		if a.observer != nil {
			a.observer.Admitted(p, 0)
		}
		return a.held(), nil
	}

	// Otherwise wait in line at the stream's priority.
	granted := make(chan struct{})
	a.waiting[p] = append(a.waiting[p], granted)
	a.mtx.Unlock()
	if a.observer != nil {
		a.observer.Waiting(p, 1)
		defer a.observer.Waiting(p, -1)
	}
	start := time.Now()
	select {
	case <-granted:
		if a.observer != nil {
			a.observer.Admitted(p, time.Since(start))
		}
		return a.held(), nil
	case <-ctx.Done():
	}

	// Leave the line; a slot granted meanwhile passes to the next stream.
	a.mtx.Lock()
	for i, w := range a.waiting[p] {
		if w == granted {
			a.waiting[p] = append(a.waiting[p][:i], a.waiting[p][i+1:]...)
			a.mtx.Unlock()
			return nil, ctx.Err()
		}
	}
	a.mtx.Unlock()
	a.release()
	return nil, ctx.Err()
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

// release grants a slot to the first waiting stream of the highest priority,
// or frees it.
func (a *Admission) release() {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	for p := range a.waiting {
		if len(a.waiting[p]) != 0 {
			close(a.waiting[p][0])
			a.waiting[p] = a.waiting[p][1:]
			return
		}
	}
	a.free++
}
