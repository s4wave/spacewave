package spacewave_cli

import (
	"testing"
	"time"
)

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
	time.Sleep(20 * time.Millisecond)
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
