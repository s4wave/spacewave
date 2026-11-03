package world

import (
	"context"
	"sync"
)

// WatchSlot keeps one watcher's change feed subscription across the
// WatchIndexedChanges calls made, one at a time, with a context that carries
// it. A view's reruns make such calls, each naming nearly the same reads, so a
// feed that keeps the subscription diffs its index entry instead of removing
// and adding every name on each rerun. A feed that does not know the slot
// ignores it.
type WatchSlot struct {
	// mtx guards the fields below.
	mtx sync.Mutex
	// busy is set while a call holds the slot.
	busy bool
	// closed is set once Close ran; a call returning after it releases what it
	// kept.
	closed bool
	// held is the kept subscription, or nil.
	held WatchSlotHolder
}

// WatchSlotHolder is a subscription a change feed keeps in a WatchSlot.
type WatchSlotHolder interface {
	// ReleaseWatch ends the subscription.
	ReleaseWatch()
}

// watchSlotKey is the context key of a WatchSlot.
type watchSlotKey struct{}

// WithWatchSlot returns ctx carrying slot for the change feed calls made
// with it.
func WithWatchSlot(ctx context.Context, slot *WatchSlot) context.Context {
	return context.WithValue(ctx, watchSlotKey{}, slot)
}

// WatchSlotFromContext returns the WatchSlot ctx carries, or nil.
func WatchSlotFromContext(ctx context.Context) *WatchSlot {
	slot, _ := ctx.Value(watchSlotKey{}).(*WatchSlot)
	return slot
}

// Take claims the slot for one call and returns the subscription it kept,
// which may be nil. ok is false when another call holds the slot or it is
// closed; the caller then subscribes on its own.
func (s *WatchSlot) Take() (held WatchSlotHolder, ok bool) {
	// Claim the kept subscription for one caller.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.busy || s.closed {
		return nil, false
	}
	s.busy = true
	held, s.held = s.held, nil
	return held, true
}

// Put ends the call that took the slot and keeps held, which may be nil, for
// the next call. After Close it releases held instead.
func (s *WatchSlot) Put(held WatchSlotHolder) {
	// Return the subscription unless the slot has already closed.
	s.mtx.Lock()
	s.busy = false
	if !s.closed {
		s.held = held
		held = nil
	}
	s.mtx.Unlock()
	if held != nil {
		held.ReleaseWatch()
	}
}

// Close releases the kept subscription, and any a running call puts back.
func (s *WatchSlot) Close() {
	// Detach the kept subscription before releasing it unlocked.
	s.mtx.Lock()
	s.closed = true
	held := s.held
	s.held = nil
	s.mtx.Unlock()
	if held != nil {
		held.ReleaseWatch()
	}
}
