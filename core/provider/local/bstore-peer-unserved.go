package provider_local

import (
	"context"
	"sync"
	"time"
)

// peerUnservedFor is how long a block the peers refused is answered
// unavailable without asking them again. A peer that answers a block with its
// own lookup error, rather than with a miss, has a tier of its own that cannot
// read it, and a read that asks again waits out the whole peer read bound
// before it fails the same way. A failed exchange is never remembered. Ten minutes outlasts the reconnects of
// a page left open and an old owner's restart, so a reader that keeps coming
// back fails at once instead of waiting each time, and still short enough
// that a block a peer regains is served within ten minutes of its return.
const peerUnservedFor = 10 * time.Minute

// unservedRefs remembers the refs the peers refused, as peerRefused proves,
// keyed by their marshaled string, each until peerUnservedFor after its last
// failure. It holds at most seenRefsCap refs and forgets the oldest first, so
// a forgotten ref costs one more read from the peers.
type unservedRefs struct {
	mu    sync.Mutex
	until map[string]time.Time
	ring  []string
	next  int
}

// newUnservedRefs returns an empty memory.
func newUnservedRefs() *unservedRefs {
	return &unservedRefs{until: make(map[string]time.Time)}
}

// remember records that the peers refused key at now. A nil memory
// remembers nothing.
func (u *unservedRefs) remember(key string, now time.Time) {
	// Extend a ref the memory already holds.
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, ok := u.until[key]; ok {
		u.until[key] = now.Add(peerUnservedFor)
		return
	}

	// Remember key, in place of the oldest key once the memory is full.
	if len(u.ring) < seenRefsCap {
		u.ring = append(u.ring, key)
	} else {
		delete(u.until, u.ring[u.next])
		u.ring[u.next] = key
		u.next = (u.next + 1) % seenRefsCap
	}
	u.until[key] = now.Add(peerUnservedFor)
}

// holds reports whether the peers refused key within peerUnservedFor before
// now. A nil memory holds nothing.
func (u *unservedRefs) holds(key string, now time.Time) bool {
	// Hold key until its last failure's memory expires.
	if u == nil {
		return false
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	until, ok := u.until[key]
	return ok && now.Before(until)
}

// absenceTrackedKey marks a context whose reader keeps its own memory of
// blocks no source serves, as withAbsenceTracked describes.
type absenceTrackedKey struct{}

// withAbsenceTracked marks ctx as the read of a caller that remembers the
// blocks no source serves and forgets them on the events that can make them
// servable, such as a participant grant or a peer session start, as the
// account replica copy does. Its peer reads ask the peers even for a ref the
// store's unserved memory holds, since that memory sees neither event, and
// would answer a block a new grant made servable as refused for
// peerUnservedFor.
func withAbsenceTracked(ctx context.Context) context.Context {
	return context.WithValue(ctx, absenceTrackedKey{}, true)
}

// absenceTracked reports whether withAbsenceTracked marked ctx.
func absenceTracked(ctx context.Context) bool {
	tracked, _ := ctx.Value(absenceTrackedKey{}).(bool)
	return tracked
}
