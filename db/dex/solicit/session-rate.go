package dex_solicit

import (
	"math"
	"sync/atomic"
	"time"
)

// probeEvery sets how often a peer's picks probe: one run of probeRun picks in
// probeEvery goes to a session other than the fastest, each in turn, so a path
// that recovers is measured again.
const probeEvery = 16

// probeRun is how many consecutive picks a probe takes. A run keeps as many
// requests outstanding on the probed session as the fastest one carries, so
// both are measured at the same depth; single wants would measure the probed
// session's round trip instead of its rate.
const probeRun = 16

// rateWeight is the weight of each new sample in a smoothed delivery rate.
const rateWeight = 0.25

// minRateSample is the shortest interval a rate sample spans. A shorter one
// measures the scheduler rather than the link, so it keeps accumulating.
const minRateSample = time.Millisecond

// deliveryRate measures how fast a session delivers while requests on it are
// outstanding. Idle time is not measured: a session nobody asks keeps the
// rate it last showed.
type deliveryRate struct {
	// received counts the bytes the session read from its stream.
	received atomic.Uint64
	// bps is the smoothed rate in bytes per second as float64 bits, zero
	// before the first sample. It is read without the session's mtx.
	bps atomic.Uint64
	// start is when the current sample began, zero while nothing is
	// outstanding. Guarded by the session's mtx.
	start time.Time
	// startBytes is received at start. Guarded by the session's mtx.
	startBytes uint64
}

// begin starts a sample unless one runs, when a request goes outstanding.
// The caller holds the session's mtx.
func (r *deliveryRate) begin(now time.Time) {
	if r.start.IsZero() {
		r.start, r.startBytes = now, r.received.Load()
	}
}

// sample folds the bytes read since the sample began into the rate when an
// answer arrives, and starts the next sample, or stops sampling when nothing
// is outstanding. The caller holds the session's mtx.
func (r *deliveryRate) sample(now time.Time, outstanding bool) {
	// Fold a long enough sample into the smoothed rate.
	if r.start.IsZero() {
		return
	}
	n := r.received.Load()
	if dt := now.Sub(r.start); dt >= minRateSample && n > r.startBytes {
		bps := float64(n-r.startBytes) / dt.Seconds()
		if prev := r.get(); prev > 0 {
			bps = prev + rateWeight*(bps-prev)
		}
		r.bps.Store(math.Float64bits(bps))
		r.start, r.startBytes = now, n
	}

	// Stop sampling while the session is idle.
	if !outstanding {
		r.start = time.Time{}
	}
}

// stop ends the running sample once nothing is outstanding. The caller holds
// the session's mtx.
func (r *deliveryRate) stop() {
	r.start = time.Time{}
}

// get returns the smoothed rate in bytes per second, or zero before the first
// sample.
func (r *deliveryRate) get() float64 {
	return math.Float64frombits(r.bps.Load())
}

// pickSession returns the session to ask of one peer's sessions, given
// oldest first: the open session with the fastest delivery rate, the newest
// among equals, so a peer with no measured session gets its newest. One run of
// probeRun picks in probeEvery goes to each other open session in turn. It
// returns nil when none is open.
func (c *Controller) pickSession(sessions []*peerSession) *peerSession {
	// Find the fastest open session and count the open ones.
	var best *peerSession
	var bestRate float64
	open := 0
	for _, s := range sessions {
		if s.closed.Load() {
			continue
		}
		open++
		if r := s.rate.get(); best == nil || r >= bestRate {
			best, bestRate = s, r
		}
	}
	if open <= 1 {
		return best
	}

	// Send a probe to the next other open session.
	n := c.picks.Add(1)
	run := n / probeRun
	if run%probeEvery != 0 {
		return best
	}
	k := int((run / probeEvery) % uint64(open-1)) //nolint:gosec // open-1 is positive and small.
	for _, s := range sessions {
		if s.closed.Load() || s == best {
			continue
		}
		if k == 0 {
			return s
		}
		k--
	}
	return best
}
