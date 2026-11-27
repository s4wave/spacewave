package s4db

import (
	"context"
	"sync"
	"sync/atomic"
)

// writerRequest waits on one queue until granted.
// All fields except ready belong to writerAdmission.mtx.
type writerRequest struct {
	// prev and next link this queued request to its neighbors.
	prev, next *writerRequest
	// ready closes once, when this request receives the writer grant.
	ready chan struct{}
	// background selects the background FIFO.
	background bool
	// granted distinguishes a concurrent grant from a queued cancellation.
	granted bool
}

// writerRequests is an intrusive FIFO with constant-work insertion and removal.
type writerRequests struct {
	// head and tail are the first and last queued requests.
	head, tail *writerRequest
}

// push appends r to the queue.
func (q *writerRequests) push(r *writerRequest) {
	// Attach the old tail before moving the endpoint.
	r.prev = q.tail
	if q.tail != nil {
		q.tail.next = r
	}
	if q.head == nil {
		q.head = r
	}
	q.tail = r
}

// remove unlinks r, which belongs to q.
func (q *writerRequests) remove(r *writerRequest) {
	// Repair the predecessor or the head endpoint.
	if r.prev != nil {
		r.prev.next = r.next
	}
	if q.head == r {
		q.head = r.next
	}

	// Repair the successor or the tail endpoint, then release the links.
	if r.next != nil {
		r.next.prev = r.prev
	}
	if q.tail == r {
		q.tail = r.prev
	}
	r.prev, r.next = nil, nil
}

// writerAdmission serializes transactions and scheduler slices on one handle.
// The file-owner lock makes this the only writable handle for the file.
// A finite foreground cohort runs before one shared background turn.
// Later foreground arrivals join the next cohort.
// A foreground head encounters at most one background slice in total,
// including a slice already running when it queued.
// Caller bodies, storage, scheduling and memtable backpressure have no time bound.
type writerAdmission struct {
	// mtx guards queue changes and grants. The mutex never covers I/O or a
	// caller body.
	mtx sync.Mutex
	// foreground and background keep FIFO order within each kind of work.
	foreground, background writerRequests
	// holder owns the one writer grant, including before its goroutine wakes.
	holder *writerRequest
	// cohort reports a captured foreground cohort, including its last holder.
	cohort bool
	// tail is the last queued member of the captured cohort, or nil once drained.
	tail *writerRequest
	// turn reserves one background slice after a completed foreground cohort.
	turn bool
	// waiting counts queued foreground requests for early background yielding.
	waiting atomic.Int32
}

// lock queues one request and waits for its grant or cancellation.
// Cancellation and dispatch inspect granted under the same mutex.
func (a *writerAdmission) lock(ctx context.Context, background bool) error {
	// Register a live request before dispatch, closing the lost-wakeup window.
	if err := ctx.Err(); err != nil {
		return err
	}
	r := &writerRequest{ready: make(chan struct{}), background: background}
	a.mtx.Lock()
	if background {
		a.background.push(r)
	}
	if !background {
		a.foreground.push(r)
		a.waiting.Add(1)
	}
	a.dispatch()
	a.mtx.Unlock()

	// A cancelled grant must be returned before reporting cancellation.
	select {
	case <-r.ready:
	case <-ctx.Done():
	}
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if err := ctx.Err(); err != nil {
		a.cancel(r)
		a.dispatch()
		return err
	}
	return nil
}

// cancel removes r or returns its grant while mtx is held.
func (a *writerAdmission) cancel(r *writerRequest) {
	// A grant has already left its queue and consumed any background turn.
	if r.granted {
		a.holder = nil
		return
	}
	if r.background {
		a.background.remove(r)
		return
	}

	// Move a cancelled cohort boundary back without admitting later arrivals.
	if a.tail == r {
		a.tail = r.prev
	}
	a.foreground.remove(r)
	a.waiting.Add(-1)
}

// dispatch grants one queue head when idle. The caller holds mtx.
func (a *writerAdmission) dispatch() {
	// Finishing the last cohort member reserves one shared background turn.
	if a.holder != nil {
		return
	}
	if a.cohort && a.tail == nil {
		a.cohort = false
		a.turn = true
	}
	if !a.cohort {
		// One background grant consumes the turn for every background caller.
		if a.background.head != nil && (a.turn || a.foreground.head == nil) {
			a.grant(&a.background)
			a.turn = false
			return
		}
		if a.foreground.head == nil {
			return
		}
		a.cohort, a.tail = true, a.foreground.tail
	}

	// Tail capture keeps later arrivals outside the finite foreground cohort.
	a.grant(&a.foreground)
}

// grant transfers the queue head to holder and wakes exactly that request.
// The caller holds mtx.
func (a *writerAdmission) grant(q *writerRequests) {
	// Detach the head, retaining an empty boundary until the last holder ends.
	r := q.head
	if a.tail == r {
		a.tail = nil
	}
	q.remove(r)
	if !r.background {
		a.waiting.Add(-1)
	}
	a.holder, r.granted = r, true
	close(r.ready)
}

// tryLock takes an optional background grant only while both queues are empty.
func (a *writerAdmission) tryLock() bool {
	// Optional work cannot overtake a queued transaction or scheduler slice.
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if a.holder != nil || a.foreground.head != nil || a.background.head != nil {
		return false
	}
	a.holder = &writerRequest{background: true, granted: true}
	a.turn = false
	return true
}

// unlock returns the holder's grant before dispatching its successor.
func (a *writerAdmission) unlock() {
	// Release and grant under one mutex, waking only the selected successor.
	a.mtx.Lock()
	a.holder = nil
	a.dispatch()
	a.mtx.Unlock()
}
