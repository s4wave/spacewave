//go:build !tinygo

package block_store_s3

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block/store/lanes"
	"github.com/sirupsen/logrus"
)

// The gate's concurrency bounds. A service account, such as a Backblaze B2
// account, throttles all of its clients together, so one gate serves every
// client of an endpoint and access key in the process.
const (
	// gateLanes is the most requests a gate ever lets in flight.
	gateLanes = 256
	// gateStart is the limit a new gate starts at.
	gateStart = 128
	// gateReserve is how many lanes of the limit refill and background
	// requests leave for urgent and interactive ones.
	gateReserve = 8
	// gateCappedFloor is how many lanes refill and background requests,
	// such as pack uploads and eviction confirms, may hold ahead of
	// foreground ones, so they progress under any foreground load.
	gateCappedFloor = 2
	// gateFloor is the lowest limit a throttle halves the gate to: the
	// reserve for foreground requests plus the capped floor.
	gateFloor = gateReserve + gateCappedFloor
	// gateDecreaseEvery is the least time between two halvings, so a burst
	// of refusals of requests sent together counts as one signal.
	gateDecreaseEvery = time.Second
	// gateBackoffFirst is the bound of the backoff after the first throttle
	// of a run.
	gateBackoffFirst = 250 * time.Millisecond
	// gatePauseMin is the shortest pause a throttle starts, even right after
	// a clean run reset the backoff.
	gatePauseMin = 250 * time.Millisecond
	// gateProbeEvery is how long a pause waits after its last throttle
	// before it lets one urgent request through as a probe.
	gateProbeEvery = time.Second
)

// gateBackoffMax caps the backoff, and with it a pause the service's
// Retry-After does not lengthen.
var gateBackoffMax = 8 * time.Second

// RequestGate admits a service account's requests by read class and adapts
// how many it keeps in flight to the service's throttling.
//
// The limit grows additively and shrinks multiplicatively: each run of limit
// requests answered without a throttle adds one lane, up to gateLanes, and a
// throttled answer halves it, at most once a gateDecreaseEvery, down to
// gateFloor. A throttle also pauses the gate until the longest of the
// service's Retry-After, gatePauseMin and a jittered backoff that doubles
// with each halving, up to gateBackoffMax, and resets after a clean run.
// While paused, the throttled requests queue again in their class behind the
// pause, so the account sees a quiet period and then a reduced rate. The one
// exception is a probe: gateProbeEvery after the last throttle, one urgent
// request may go, and a clean answer to it ends the pause early at the
// current limit, while a throttle extends the pause as any other does.
//
// Lock order: bcast, then the pool's lock, which is a leaf.
type RequestGate struct {
	// endpoint is the service endpoint.
	endpoint string
	// account is a short hash of the access key ID.
	account string
	// le logs the requests that give up.
	le *logrus.Entry
	// pool holds the request lanes.
	pool *lanes.Pool[struct{}]
	// resume ends a pause at its deadline.
	resume *time.Timer
	// probe lets an urgent probe through a pause.
	probe *time.Timer

	// bcast guards the fields below and broadcasts each change of the
	// limit or the pause.
	bcast broadcast.Broadcast
	// limit is the adapted limit, which the pool holds unless paused.
	limit int
	// pausedUntil is when the current pause ends, or zero.
	pausedUntil time.Time
	// lastDecrease is when the limit last halved.
	lastDecrease time.Time
	// clean counts the requests answered without a throttle since the
	// limit last changed.
	clean int
	// backoff is the bound of the next pause's jitter, zero after a clean
	// run.
	backoff time.Duration
	// throttles counts the throttled answers.
	throttles uint64
	// retries counts the retries of requests that failed in transit or with
	// a transient fault.
	retries uint64
}

// gates holds the process's request gates by endpoint and access key.
var gates struct {
	// mtx guards m.
	mtx sync.Mutex
	// m holds the gates by gateKey.
	m map[string]*RequestGate
}

// requestGate returns the process's gate for an endpoint and access key,
// creating it on first use.
func requestGate(endpoint, accessKey string) *RequestGate {
	// Return the account's gate, or add one.
	key := endpoint + "\x00" + accessKey
	gates.mtx.Lock()
	defer gates.mtx.Unlock()
	if g := gates.m[key]; g != nil {
		return g
	}
	if gates.m == nil {
		gates.m = make(map[string]*RequestGate)
	}

	// Name the account by a short hash of its key ID, never the ID.
	sum := sha256.Sum256([]byte(accessKey))
	g := newRequestGate(endpoint, hex.EncodeToString(sum[:4]))
	gates.m[key] = g
	return g
}

// VisitRequestGates calls fn with the state of each of the process's request
// gates, for metrics.
func VisitRequestGates(fn func(GateState)) {
	// Copy the gates, then read each without the registry lock.
	gates.mtx.Lock()
	list := make([]*RequestGate, 0, len(gates.m))
	for _, g := range gates.m {
		list = append(list, g)
	}
	gates.mtx.Unlock()
	slices.SortFunc(list, func(a, b *RequestGate) int {
		return cmp.Or(cmp.Compare(a.endpoint, b.endpoint), cmp.Compare(a.account, b.account))
	})
	for _, g := range list {
		st, _ := g.State()
		fn(st)
	}
}

// newRequestGate builds an unpaused gate at the starting limit for an
// endpoint and the short hash of an account.
func newRequestGate(endpoint, account string) *RequestGate {
	// Build the lanes at the starting limit, and stopped pause timers.
	g := &RequestGate{
		endpoint: endpoint,
		account:  account,
		le: logrus.WithFields(logrus.Fields{
			"s3-endpoint": endpoint,
			"s3-account":  account,
		}),
		pool:  lanes.New(make([]struct{}, gateLanes), gateReserve),
		limit: gateStart,
	}
	g.pool.SetLimit(gateStart)
	g.pool.SetCappedFloor(gateCappedFloor)
	g.resume = time.AfterFunc(time.Hour, g.endPause)
	g.resume.Stop()
	g.probe = time.AfterFunc(time.Hour, g.openProbe)
	g.probe.Stop()
	return g
}

// State returns a snapshot of the gate and a channel closed at its next
// change of limit or pause.
func (g *RequestGate) State() (GateState, <-chan struct{}) {
	var st GateState
	var ch <-chan struct{}
	g.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ps := g.pool.Stats()
		st = GateState{
			Endpoint:          g.endpoint,
			Account:           g.account,
			Limit:             g.limit,
			PausedUntil:       g.pausedUntil,
			InFlight:          ps.Held,
			QueuedUrgent:      ps.Urgent,
			QueuedInteractive: ps.Interactive,
			QueuedRefill:      ps.Refill,
			QueuedBackground:  ps.Background,
			Throttles:         g.throttles,
			Retries:           g.retries,
		}
		ch = getWaitCh()
	})
	return st, ch
}

// throttled records a throttled answer whose Retry-After asked for wait:
// halves the limit unless it halved within gateDecreaseEvery, pauses the gate
// until the longest of wait, gatePauseMin and a jittered backoff, and holds
// the next probe for gateProbeEvery.
func (g *RequestGate) throttled(wait time.Duration) {
	g.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Halve the limit and double the backoff once per decrease window.
		now := time.Now()
		g.throttles++
		g.clean = 0
		if now.Sub(g.lastDecrease) >= gateDecreaseEvery {
			g.lastDecrease = now
			g.limit = max(g.limit/2, gateFloor)
			g.backoff = min(max(g.backoff*2, gateBackoffFirst), gateBackoffMax)
		}

		// Extend the pause to the longest of the service's wait, the
		// minimum and the jittered backoff, and restart the probe's wait.
		pause := max(wait, gatePauseMin, g.backoff/2+jitter(g.backoff/2))
		until := now.Add(pause)
		if until.After(g.pausedUntil) {
			g.pausedUntil = until
			g.pool.SetLimit(0)
			g.resume.Reset(pause)
		}
		g.pool.SetProbes(0)
		g.probe.Reset(gateProbeEvery)
		broadcast()
	})
}

// endPause runs at a pause's deadline: it restores the limit unless a later
// throttle extended the pause, in which case it waits again.
func (g *RequestGate) endPause() {
	g.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Wait again for an extended pause, else restore the limit.
		if g.pausedUntil.IsZero() {
			return
		}
		if left := time.Until(g.pausedUntil); left > 0 {
			g.resume.Reset(left)
			return
		}
		g.resumeLocked()
		broadcast()
	})
}

// openProbe runs gateProbeEvery after a pause's last throttle: it lets one
// urgent request through while the pause lasts.
func (g *RequestGate) openProbe() {
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !g.pausedUntil.IsZero() {
			g.pool.SetProbes(1)
		}
	})
}

// probeAnswered records a probe the service answered without a throttle: it
// ends the pause early at the current limit.
func (g *RequestGate) probeAnswered() {
	g.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if g.pausedUntil.IsZero() {
			return
		}
		g.resume.Stop()
		g.resumeLocked()
		broadcast()
	})
}

// resumeLocked ends the pause: it closes the probe and restores the limit.
func (g *RequestGate) resumeLocked() {
	g.pausedUntil = time.Time{}
	g.probe.Stop()
	g.pool.SetProbes(0)
	g.pool.SetLimit(g.limit)
}

// answered records a request the service answered without a throttle. Each
// run of limit such answers adds a lane and resets the backoff.
func (g *RequestGate) answered() {
	g.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Count the answer, and add a lane at the end of a clean run.
		g.clean++
		if g.clean < g.limit {
			return
		}
		g.clean = 0
		g.backoff = 0
		if g.limit >= gateLanes {
			return
		}
		g.limit++
		if g.pausedUntil.IsZero() {
			g.pool.SetLimit(g.limit)
		}
		broadcast()
	})
}

// retried counts a retry of a transient fault.
func (g *RequestGate) retried() {
	g.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		g.retries++
	})
}
