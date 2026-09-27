//go:build !js

package spacewave_cli

import (
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
)

const daemonIdleTimeoutEnvVar = "SPACEWAVE_DAEMON_IDLE_TIMEOUT"

var defaultDaemonIdleTimeout = 30 * time.Second

// daemonIdleSnapshot reports the holds and revision of the daemon's lifetime.
type daemonIdleSnapshot struct {
	// clients is the number of admitted public connections.
	clients int
	// services is the number of persistent service holds.
	services int
	// revision identifies this coherent state and its next change event.
	revision uint64
	// stopping indicates that no new hold can be admitted.
	stopping bool
}

// daemonServiceHold identifies one persistent service in the idle tracker.
type daemonServiceHold struct {
	// tracker owns the service count and guards released.
	tracker *daemonIdleTracker
	// released prevents this hold from releasing another service.
	released bool
}

// release removes this exact service hold once.
func (h *daemonServiceHold) release() {
	t := h.tracker
	t.mu.Lock()
	defer t.mu.Unlock()
	if h.released {
		return
	}

	h.released = true
	t.services--
	t.active--
	t.publishLocked()
	t.armIdleLocked()
}

// daemonIdleTracker owns client admission, service holds, and idle shutdown.
type daemonIdleTracker struct {
	// mu guards the holds, event, deadline, and stop claim.
	mu sync.Mutex
	// active is the total number of client and service holds.
	active int
	// clients counts admitted public socket connections.
	clients int
	// connections identifies admitted socket clients for requester exclusion.
	connections map[*trackedConn]struct{}
	// services counts persistent daemon services.
	services int
	// revision increases on each observable state change.
	revision uint64
	// changed closes when revision increases.
	changed chan struct{}
	// stopping rejects new holds after shutdown is claimed.
	stopping bool
	// idleTimer expires after the final hold leaves.
	idleTimer *time.Timer
	// idleGeneration invalidates canceled deadline callbacks.
	idleGeneration uint64
	// idleTimeout is the final-release deadline.
	idleTimeout time.Duration
	// onIdle requests shutdown after expiry is claimed.
	onIdle func()
}

// newDaemonIdleTracker constructs a daemon idle tracker.
func newDaemonIdleTracker(idleTimeout time.Duration, onIdle func()) *daemonIdleTracker {
	return &daemonIdleTracker{
		changed:     make(chan struct{}),
		connections: make(map[*trackedConn]struct{}),
		idleTimeout: idleTimeout,
		onIdle:      onIdle,
	}
}

// observe returns a coherent snapshot and the channel closed by its next change.
func (t *daemonIdleTracker) observe() (daemonIdleSnapshot, <-chan struct{}) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked(), t.changed
}

// clientAttached admits a public client unless shutdown has been claimed.
// An accepted socket rejected here must be closed without serving it.
func (t *daemonIdleTracker) clientAttached() bool {
	return t.trackedClientAttached(nil)
}

// trackedClientAttached admits a socket and records its identity for a later
// stop claim by that connection.
func (t *daemonIdleTracker) trackedClientAttached(conn *trackedConn) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopping {
		return false
	}

	// Cancel the deadline and publish the new client hold.
	t.cancelIdleLocked()
	t.clients++
	if conn != nil {
		t.connections[conn] = struct{}{}
	}
	t.active++
	t.publishLocked()
	return true
}

// clientDetached releases a previously admitted public client.
func (t *daemonIdleTracker) clientDetached() {
	t.trackedClientDetached(nil)
}

// trackedClientDetached releases an admitted socket and its claim identity.
func (t *daemonIdleTracker) trackedClientDetached(conn *trackedConn) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if conn != nil {
		if _, ok := t.connections[conn]; !ok {
			return
		}
		delete(t.connections, conn)
	}
	if t.clients == 0 {
		return
	}
	t.clients--
	t.active--
	t.publishLocked()
	t.armIdleLocked()
}

// serviceAttached holds the daemon for a persistent service and returns an
// idempotent release callback.
func (t *daemonIdleTracker) serviceAttached() func() {
	return t.attachService().release
}

// attachService returns the identity and release operation for one service.
func (t *daemonIdleTracker) attachService() *daemonServiceHold {
	t.mu.Lock()
	defer t.mu.Unlock()
	hold := &daemonServiceHold{tracker: t}
	if t.stopping {
		hold.released = true
		return hold
	}

	// Cancel the deadline while the persistent service is active.
	t.cancelIdleLocked()
	t.services++
	t.active++
	t.publishLocked()

	return hold
}

// claimDesktopQuit decides the final busy result while Electron can report it.
// Only the identified requester and live desktop hold are excluded. A winning
// claim fences later admission; teardown waits for the shell and RPC reply.
func (t *daemonIdleTracker) claimDesktopQuit(requester *trackedConn, desktop *daemonServiceHold) (bool, daemonIdleSnapshot) {
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := t.snapshotLocked()
	if t.stopping || desktop == nil || desktop.tracker != t || desktop.released {
		return false, snapshot
	}
	if _, ok := t.connections[requester]; requester != nil && ok {
		snapshot.clients--
	}
	snapshot.services--
	if snapshot.clients != 0 || snapshot.services != 0 {
		return false, snapshot
	}

	t.stopping = true
	t.cancelIdleLocked()
	t.publishLocked()
	snapshot.stopping = true
	return true, snapshot
}

// snapshotLocked returns the current state while mu is held.
func (t *daemonIdleTracker) snapshotLocked() daemonIdleSnapshot {
	return daemonIdleSnapshot{
		clients:  t.clients,
		services: t.services,
		revision: t.revision,
		stopping: t.stopping,
	}
}

// publishLocked advances the revision and wakes subscribers while mu is held.
func (t *daemonIdleTracker) publishLocked() {
	t.revision++
	close(t.changed)
	t.changed = make(chan struct{})
}

// cancelIdleLocked invalidates and stops the pending deadline while mu is held.
func (t *daemonIdleTracker) cancelIdleLocked() {
	t.idleGeneration++
	if t.idleTimer != nil {
		t.idleTimer.Stop()
		t.idleTimer = nil
	}
}

// armIdleLocked schedules expiry only after a transition to no active holds.
func (t *daemonIdleTracker) armIdleLocked() {
	if t.active != 0 || t.stopping || t.idleTimeout <= 0 || t.onIdle == nil {
		return
	}
	generation := t.idleGeneration
	t.idleTimer = time.AfterFunc(t.idleTimeout, func() {
		t.mu.Lock()
		if t.stopping || t.active != 0 || t.idleGeneration != generation {
			t.mu.Unlock()
			return
		}

		// Claim expiry before invoking shutdown so admission cannot revive it.
		t.stopping = true
		t.idleTimer = nil
		t.publishLocked()
		t.mu.Unlock()
		t.onIdle()
	})
}

// close cancels the deadline and rejects later attachments during teardown.
func (t *daemonIdleTracker) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopping {
		return
	}
	t.stopping = true
	t.cancelIdleLocked()
	t.publishLocked()
}

// getDaemonIdleTimeout returns the configured idle timeout.
func getDaemonIdleTimeout() (time.Duration, error) {
	raw := os.Getenv(daemonIdleTimeoutEnvVar)
	if raw == "" {
		return defaultDaemonIdleTimeout, nil
	}
	dur, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.Wrap(err, daemonIdleTimeoutEnvVar)
	}
	return dur, nil
}
