package resource_proxy

import "sync/atomic"

// Limit caps the unary calls that the proxies sharing it hold while they wait
// for an answer: calls waiting for an upstream, and calls an upstream has not
// answered yet. A call past the cap ends at once with srpc's retryable reset,
// as it would on a dropped connection, so what a stalled upstream costs the
// proxy stays bounded and the client's own retry decides what happens next.
// Streams are not counted: a client holds a few of them for its whole life.
type Limit struct {
	// max is the cap.
	max int64
	// full is called for each call past the cap, or nil.
	full func()
	// held counts the calls held now.
	held atomic.Int64
}

// NewLimit constructs a limit of max held calls, which calls full, when set,
// for each call it ends.
func NewLimit(max int, full func()) *Limit {
	return &Limit{max: int64(max), full: full}
}

// take counts one more held call, or reports false at the cap.
func (l *Limit) take() bool {
	if l.held.Add(1) > l.max {
		l.held.Add(-1)
		if l.full != nil {
			l.full()
		}
		return false
	}
	return true
}

// put releases a call take counted.
func (l *Limit) put() {
	l.held.Add(-1)
}

// takeAll takes one call on each of limits, or none when one is at its cap.
// It returns the function that puts them back.
func takeAll(limits []*Limit) (func(), bool) {
	for i, l := range limits {
		if !l.take() {
			for _, taken := range limits[:i] {
				taken.put()
			}
			return nil, false
		}
	}
	return func() {
		for _, l := range limits {
			l.put()
		}
	}, true
}

// unary reports whether calls of r are unary, which a Limit counts.
func (r Rule) unary() bool {
	switch r {
	case Retry, Once, Replay, Open:
		return true
	default:
		return false
	}
}
