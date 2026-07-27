//go:build !js

package spacewave_cli

import (
	"sync"
	"time"
)

const daemonIdleTimeoutEnvVar = "SPACEWAVE_DAEMON_IDLE_TIMEOUT"

var defaultDaemonIdleTimeout = 30 * time.Second

// daemonIdleTracker starts an idle shutdown timer when the active-work count reaches zero.
type daemonIdleTracker struct {
	mu          sync.Mutex
	active      int
	idleTimer   *time.Timer
	idleTimeout time.Duration
	onIdle      func()
}

// newDaemonIdleTracker constructs a daemon idle tracker.
func newDaemonIdleTracker(idleTimeout time.Duration, onIdle func()) *daemonIdleTracker {
	return &daemonIdleTracker{
		idleTimeout: idleTimeout,
		onIdle:      onIdle,
	}
}

// clientAttached increments the active client count and clears any idle timer.
func (t *daemonIdleTracker) clientAttached() {
	t.activeAttached()
}

// serviceAttached increments the persistent service count and returns a release callback.
func (t *daemonIdleTracker) serviceAttached() func() {
	t.activeAttached()

	var once sync.Once
	return func() {
		once.Do(t.activeDetached)
	}
}

func (t *daemonIdleTracker) activeAttached() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.active++
	if t.idleTimer != nil {
		t.idleTimer.Stop()
		t.idleTimer = nil
	}
}

// clientDetached decrements the active client count and starts the idle timer on a 1->0 transition.
func (t *daemonIdleTracker) clientDetached() {
	t.activeDetached()
}

func (t *daemonIdleTracker) activeDetached() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.active == 0 {
		return
	}
	t.active--
	if t.active != 0 || t.idleTimeout <= 0 || t.onIdle == nil {
		return
	}
	t.idleTimer = time.AfterFunc(t.idleTimeout, t.onIdle)
}

// close stops any pending idle timer.
func (t *daemonIdleTracker) close() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.idleTimer != nil {
		t.idleTimer.Stop()
		t.idleTimer = nil
	}
}
