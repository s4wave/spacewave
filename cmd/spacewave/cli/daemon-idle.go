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
	// desktop indicates that the desktop shell holds the daemon.
	desktop bool
}

// daemonServiceHold identifies one persistent service in the idle tracker.
type daemonServiceHold struct {
	// tracker owns the service count and guards released.
	tracker *daemonIdleTracker
	// released prevents this hold from releasing another service.
	released bool
}

// release removes this exact service hold once.
// Mark the hold released and update the tracker under its lock.
func (h *daemonServiceHold) release() {
	// Take the tracker lock for this hold.
	t := h.tracker
	t.mu.Lock()
	defer t.mu.Unlock()
	if h.released {
		return
	}

	// Clear the hold and rearm idle shutdown.
	h.released = true
	if t.desktop == h {
		t.desktop = nil
	}
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
	// desktop identifies the desktop shell's service hold, if any.
	desktop *daemonServiceHold
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
	// Take the tracker lock and reject when shutdown is claimed.
	t.mu.Lock()
	defer t.mu.Unlock()

	// Reject attachment when shutdown has been claimed.
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
	// Take the tracker lock and remove the connection hold.
	t.mu.Lock()

	// Remove the tracked connection and release its client hold.
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

	// Release the client hold and rearm idle shutdown.
	t.armIdleLocked()
}

// serviceAttached holds the daemon for a persistent service and returns an
// idempotent release callback.
func (t *daemonIdleTracker) serviceAttached() func() {
	return t.attachService().release
}

// attachService returns the identity and release operation for one service.
// Take the tracker lock and create the hold.
func (t *daemonIdleTracker) attachService() *daemonServiceHold {
	// Take the tracker lock and create the hold.
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create the hold and reject it when stopping.
	hold := &daemonServiceHold{tracker: t}
	if t.stopping {
		hold.released = true
		return hold
	}

	// Cancel the deadline while the persistent service is active.
	// Count the service and publish the new hold.
	t.cancelIdleLocked()
	t.services++
	t.active++
	t.publishLocked()

	return hold
}

// setDesktop marks a live service hold as the desktop shell's hold until it
// is released. Stop decisions for the desktop exclude that hold.
func (t *daemonIdleTracker) setDesktop(hold *daemonServiceHold) {
	// Mark the hold as the desktop shell under the tracker lock.
	t.mu.Lock()
	defer t.mu.Unlock()
	if hold.tracker != t || hold.released || t.desktop == hold {
		return
	}
	t.desktop = hold
	t.publishLocked()
}

// claimDesktopQuit decides the final busy result while Electron can report it.
// Only the identified requester and live desktop hold are excluded. A winning
// claim fences later admission; teardown waits for the shell and RPC reply.
func (t *daemonIdleTracker) claimDesktopQuit(requester *trackedConn) (bool, daemonIdleSnapshot) {
	// Take the tracker lock for the claim decision.
	t.mu.Lock()
	defer t.mu.Unlock()

	// Snapshot the other holds.
	snapshot := t.otherHoldsLocked()

	// Reject the claim when stopping or the desktop is gone.
	if t.stopping || !snapshot.desktop {
		return false, snapshot
	}

	// Exclude the requester from its own client hold.
	if _, ok := t.connections[requester]; requester != nil && ok {
		snapshot.clients--
	}

	// Reject the claim while other holds remain.
	if snapshot.clients != 0 || snapshot.services != 0 {
		return false, snapshot
	}

	// Claim shutdown and report success.
	t.claimStopLocked()
	snapshot.stopping = true
	return true, snapshot
}

// claimDaemonUpdate fences admission for an accepted update once every hold
// except the desktop shell is released, or at once when restartNow is set.
// The handoff reopens the desktop. The snapshot counts only the other holds,
// and the returned channel closes on the tracker's next change.
// Take the tracker lock for the claim decision.
func (t *daemonIdleTracker) claimDaemonUpdate(restartNow bool) (bool, daemonIdleSnapshot, <-chan struct{}) {
	// Take the tracker lock for the claim decision.
	t.mu.Lock()
	defer t.mu.Unlock()

	// Snapshot the other holds and reject while they remain.
	snapshot := t.otherHoldsLocked()
	if t.stopping || (!restartNow && (snapshot.clients != 0 || snapshot.services != 0)) {
		return false, snapshot, t.changed
	}

	// Claim shutdown and report success.
	t.claimStopLocked()
	snapshot.stopping = true
	return true, snapshot, t.changed
}

// otherHoldsLocked returns the current state without the desktop hold while
// mu is held.
func (t *daemonIdleTracker) otherHoldsLocked() daemonIdleSnapshot {
	snapshot := t.snapshotLocked()
	if snapshot.desktop {
		snapshot.services--
	}
	return snapshot
}

// claimStopLocked fences admission and cancels the deadline while mu is held.
func (t *daemonIdleTracker) claimStopLocked() {
	t.stopping = true
	t.cancelIdleLocked()
	t.publishLocked()
}

// snapshotLocked returns the current state while mu is held.
func (t *daemonIdleTracker) snapshotLocked() daemonIdleSnapshot {
	return daemonIdleSnapshot{
		clients:  t.clients,
		services: t.services,
		revision: t.revision,
		stopping: t.stopping,
		desktop:  t.desktop != nil,
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
		// Take the tracker lock and check the idle generation.
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
	t.claimStopLocked()
}

// getDaemonIdleTimeout returns the configured idle timeout.
func getDaemonIdleTimeout() (time.Duration, error) {
	// Read the idle timeout from the environment.
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
