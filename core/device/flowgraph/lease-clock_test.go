//go:build !js

package device_flowgraph

import (
	"sync"
	"time"
)

// leaseClock advances lease deadlines without sleeping or writing to the World.
type leaseClock struct {
	// mtx guards the clock and registered deadlines.
	mtx sync.Mutex
	// now is the current lease time.
	now time.Time
	// timers holds cancellable deadlines.
	timers map[*leaseTimer]struct{}
}

// leaseTimer stores a callback until its deadline or cancellation.
type leaseTimer struct {
	// at is the deadline in lease time.
	at time.Time
	// fire wakes the lease reader.
	fire func()
}

// Now returns the current lease time.
func (c *leaseClock) Now() time.Time {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	return c.now
}

// AtFunc registers a deadline and returns its cancellation.
func (c *leaseClock) AtFunc(deadline time.Time, fire func()) func() {
	// Register while holding the same lock that advances the clock.
	c.mtx.Lock()
	timer := &leaseTimer{at: deadline, fire: fire}
	due := !c.now.Before(timer.at)
	if !due {
		c.timers[timer] = struct{}{}
	}
	c.mtx.Unlock()

	// An already expired deadline wakes the reader immediately.
	if due {
		fire()
	}
	return func() {
		c.mtx.Lock()
		delete(c.timers, timer)
		c.mtx.Unlock()
	}
}

// Advance moves time forward and fires each due deadline once outside the lock.
func (c *leaseClock) Advance(elapsed time.Duration) {
	// Detach due timers before invoking callbacks that may read this clock.
	c.mtx.Lock()
	c.now = c.now.Add(elapsed)
	due := make([]func(), 0, len(c.timers))
	for timer := range c.timers {
		if !c.now.Before(timer.at) {
			due = append(due, timer.fire)
			delete(c.timers, timer)
		}
	}
	c.mtx.Unlock()

	// Notify the production placement readers without changing the World.
	for _, fire := range due {
		fire()
	}
}
