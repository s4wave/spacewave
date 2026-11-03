package viewstream

import "sync"

// Writes records the World revision of each actor's latest write, so a
// view bound to that actor can tell a change it waits on carries the
// actor's own write.
type Writes struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// last maps a actor's application actor ID to the revision of their latest write.
	last map[uint64]uint64
	// wake maps a actor's application actor ID to a channel that closes at their next
	// write, for the views waiting out their interval.
	wake map[uint64]chan struct{}
}

// NewWrites constructs an empty record.
func NewWrites() *Writes {
	return &Writes{last: make(map[uint64]uint64), wake: make(map[uint64]chan struct{})}
}

// Record notes that actors wrote revision seqno and wakes their waiting views.
func (o *Writes) Record(actors []uint64, seqno uint64) {
	// Take the wake channels under the lock and close them after it, so the
	// views they ready do not wait on the lock the commit path holds.
	var wakes []chan struct{}
	o.mtx.Lock()
	for _, actor := range actors {
		o.last[actor] = max(o.last[actor], seqno)
		if wake, ok := o.wake[actor]; ok {
			wakes = append(wakes, wake)
			delete(o.wake, actor)
		}
	}
	o.mtx.Unlock()
	for _, wake := range wakes {
		close(wake)
	}
}

// Latest returns the revision of actor's latest write, or zero.
func (o *Writes) Latest(actor uint64) uint64 {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	return o.last[actor]
}

// Since reports whether actor wrote a revision after seqno, or else returns
// a channel that closes at their next write.
func (o *Writes) Since(actor, seqno uint64) (bool, <-chan struct{}) {
	// Sample the next write channel under the same lock as its revision.
	o.mtx.Lock()
	defer o.mtx.Unlock()
	if o.last[actor] > seqno {
		return true, nil
	}
	wake, ok := o.wake[actor]
	if !ok {
		wake = make(chan struct{})
		o.wake[actor] = wake
	}
	return false, wake
}
