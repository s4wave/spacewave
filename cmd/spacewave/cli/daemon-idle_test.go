//go:build !js

package spacewave_cli

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestDaemonIdleTrackerOrdinaryFinalReleaseUsesThirtySeconds verifies that a
// desktop service's ordinary release follows the common default idle expiry.
func TestDaemonIdleTrackerOrdinaryFinalReleaseUsesThirtySeconds(t *testing.T) {
	// Give the desktop one persistent service hold under the shipped timeout.
	idleCh := make(chan struct{}, 1)
	tracker := newDaemonIdleTracker(defaultDaemonIdleTimeout, func() {
		idleCh <- struct{}{}
	})
	t.Cleanup(tracker.close)
	releaseDesktop := tracker.serviceAttached()

	// Observe the owner's expiry event after ordinary final service release.
	start := time.Now()
	releaseDesktop()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	select {
	case <-idleCh:
	case <-ctx.Done():
		t.Fatal("idle expiry did not fire after final desktop release")
	}
	if elapsed := time.Since(start); elapsed < 30*time.Second {
		t.Fatalf("ordinary idle expiry after %s, want at least 30s", elapsed)
	}
}

// TestDaemonIdleTrackerClaimDesktopQuit excludes only the identified live holds.
func TestDaemonIdleTrackerClaimDesktopQuit(t *testing.T) {
	tracker := newDaemonIdleTracker(0, nil)
	t.Cleanup(tracker.close)
	requester := &trackedConn{}
	tracker.trackedClientAttached(requester)
	tracker.clientAttached()
	t.Cleanup(tracker.clientDetached)
	desktop := tracker.attachService()
	releaseOther := tracker.serviceAttached()
	t.Cleanup(releaseOther)
	t.Cleanup(desktop.release)

	// Count only the client and service outside the requesting desktop.
	claimed, snapshot := tracker.claimDesktopQuit(requester, desktop)
	if claimed || snapshot.clients != 1 || snapshot.services != 1 {
		t.Fatalf("admitted requester demand = %+v", snapshot)
	}

	// A disconnected requester must not subtract a different live client.
	tracker.trackedClientDetached(requester)
	claimed, snapshot = tracker.claimDesktopQuit(requester, desktop)
	if claimed || snapshot.clients != 1 || snapshot.services != 1 {
		t.Fatalf("disconnected requester demand = %+v", snapshot)
	}
}

// TestDaemonIdleTrackerDesktopClaimFencesAdmission verifies that a successful
// claim precedes shell exit and rejects a client racing with that claim.
func TestDaemonIdleTrackerDesktopClaimFencesAdmission(t *testing.T) {
	for range 100 {
		tracker := newDaemonIdleTracker(time.Minute, nil)
		requester := &trackedConn{}
		tracker.trackedClientAttached(requester)
		desktop := tracker.attachService()
		start := make(chan struct{})
		var workers sync.WaitGroup
		var admitted, claimed bool
		var snapshot daemonIdleSnapshot
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			admitted = tracker.clientAttached()
		}()
		go func() {
			defer workers.Done()
			<-start
			claimed, snapshot = tracker.claimDesktopQuit(requester, desktop)
		}()
		close(start)
		workers.Wait()
		if admitted == claimed {
			t.Fatalf("admitted = %t, claimed = %t, state = %+v", admitted, claimed, snapshot)
		}
		if admitted && (snapshot.clients != 1 || snapshot.services != 0) {
			t.Fatalf("busy decision = %+v, want exactly one other client", snapshot)
		}
		desktop.release()
		tracker.close()
	}
}

// TestDaemonIdleTrackerReportsHoldsAndChanges exercises the coherent snapshot
// and event across both kinds of hold.
func TestDaemonIdleTrackerReportsHoldsAndChanges(t *testing.T) {
	tracker := newDaemonIdleTracker(time.Minute, func() {})
	t.Cleanup(tracker.close)

	initial, changed := tracker.observe()
	if initial.clients != 0 || initial.services != 0 {
		t.Fatalf("initial holds = %+v", initial)
	}
	if !tracker.clientAttached() {
		t.Fatal("client rejected before stop")
	}
	select {
	case <-changed:
	default:
		t.Fatal("client attachment did not publish change")
	}

	client, changed := tracker.observe()
	if client.clients != 1 || client.services != 0 || client.revision != initial.revision+1 {
		t.Fatalf("client snapshot = %+v", client)
	}
	release := tracker.serviceAttached()
	select {
	case <-changed:
	default:
		t.Fatal("service attachment did not publish change")
	}

	busy, changed := tracker.observe()
	if busy.clients != 1 || busy.services != 1 {
		t.Fatalf("busy snapshot = %+v", busy)
	}
	tracker.clientDetached()
	select {
	case <-changed:
	default:
		t.Fatal("client release did not publish change")
	}

	service, changed := tracker.observe()
	if service.clients != 0 || service.services != 1 {
		t.Fatalf("service snapshot = %+v", service)
	}
	release()
	select {
	case <-changed:
	default:
		t.Fatal("service release did not publish change")
	}

	idle, _ := tracker.observe()
	if idle.clients != 0 || idle.services != 0 || idle.revision != initial.revision+4 {
		t.Fatalf("idle snapshot = %+v", idle)
	}
}

// TestDaemonIdleTrackerDeadlinePolicy checks final-release expiry against an
// isolated short deadline and confirms a new hold cancels that deadline.
func TestDaemonIdleTrackerDeadlinePolicy(t *testing.T) {
	const deadline = 30 * time.Millisecond
	idle := make(chan time.Time, 1)
	tracker := newDaemonIdleTracker(deadline, func() { idle <- time.Now() })
	t.Cleanup(tracker.close)

	tracker.clientAttached()
	tracker.clientDetached()
	tracker.clientAttached()
	select {
	case <-idle:
		t.Fatal("expired while client retained")
	case <-time.After(deadline + 10*time.Millisecond):
	}

	releasedAt := time.Now()
	tracker.clientDetached()
	select {
	case expiredAt := <-idle:
		if elapsed := expiredAt.Sub(releasedAt); elapsed < deadline {
			t.Fatalf("expired after %v, before %v deadline", elapsed, deadline)
		}
	case <-time.After(time.Second):
		t.Fatal("idle deadline did not expire")
	}
	if tracker.clientAttached() {
		t.Fatal("client admitted after idle expiry")
	}
}

func TestGetDaemonIdleTimeoutUsesEnvironment(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "45s")

	got, err := getDaemonIdleTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if got != 45*time.Second {
		t.Fatalf("idle timeout = %v, want %v", got, 45*time.Second)
	}
}

// TestGetDaemonIdleTimeoutDefaultsToThirtySeconds preserves the common idle
// deadline used by every daemon starter.
func TestGetDaemonIdleTimeoutDefaultsToThirtySeconds(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "")

	got, err := getDaemonIdleTimeout()
	if err != nil {
		t.Fatal(err)
	}
	if got != 30*time.Second {
		t.Fatalf("idle timeout = %v, want 30s", got)
	}
}

func TestGetDaemonIdleTimeoutInvalidEnvironment(t *testing.T) {
	t.Setenv(daemonIdleTimeoutEnvVar, "not-a-duration")

	_, err := getDaemonIdleTimeout()
	if err == nil {
		t.Fatal("expected invalid idle timeout error")
	}
}

func TestDaemonIdleTrackerStartsTimerOnTransitionToZero(t *testing.T) {
	idleCh := make(chan struct{}, 1)
	tracker := newDaemonIdleTracker(25*time.Millisecond, func() {
		idleCh <- struct{}{}
	})
	defer tracker.close()

	tracker.clientAttached()
	tracker.clientDetached()

	select {
	case <-idleCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected idle callback")
	}
}

func TestDaemonIdleTrackerDoesNotStartAtInitialZero(t *testing.T) {
	idleCh := make(chan struct{}, 1)
	tracker := newDaemonIdleTracker(25*time.Millisecond, func() {
		idleCh <- struct{}{}
	})
	defer tracker.close()

	select {
	case <-idleCh:
		t.Fatal("unexpected idle callback")
	case <-time.After(75 * time.Millisecond):
	}
}

func TestDaemonIdleTrackerStopsTimerWhenClientReattaches(t *testing.T) {
	idleCh := make(chan struct{}, 1)
	tracker := newDaemonIdleTracker(50*time.Millisecond, func() {
		idleCh <- struct{}{}
	})
	defer tracker.close()

	tracker.clientAttached()
	tracker.clientDetached()
	tracker.clientAttached()

	select {
	case <-idleCh:
		t.Fatal("unexpected idle callback")
	case <-time.After(75 * time.Millisecond):
	}
}

func TestDaemonIdleTrackerWaitsForServiceRelease(t *testing.T) {
	idleCh := make(chan struct{}, 1)
	tracker := newDaemonIdleTracker(50*time.Millisecond, func() {
		idleCh <- struct{}{}
	})
	defer tracker.close()

	tracker.clientAttached()
	releaseService := tracker.serviceAttached()
	tracker.clientDetached()

	select {
	case <-idleCh:
		t.Fatal("unexpected idle callback while service active")
	case <-time.After(75 * time.Millisecond):
	}

	releaseService()

	select {
	case <-idleCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected idle callback after service release")
	}
}

func TestDaemonIdleTrackerServiceReleaseIsIdempotent(t *testing.T) {
	idleCh := make(chan struct{}, 1)
	tracker := newDaemonIdleTracker(25*time.Millisecond, func() {
		idleCh <- struct{}{}
	})
	defer tracker.close()

	releaseService := tracker.serviceAttached()
	releaseService()
	releaseService()

	select {
	case <-idleCh:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected idle callback after service release")
	}
}

func TestDaemonIdleTrackerZeroTimeoutDisablesShutdown(t *testing.T) {
	tracker := newDaemonIdleTracker(0, func() {})
	t.Cleanup(tracker.close)

	tracker.clientAttached()
	tracker.clientDetached()

	tracker.mu.Lock()
	idleTimer := tracker.idleTimer
	tracker.mu.Unlock()
	if idleTimer != nil {
		t.Fatal("idle timer armed with zero timeout")
	}
}
