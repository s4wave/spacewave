package store

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestDemandGateBackgroundWaitsForForeground verifies a background fetch waits
// while a foreground fetch runs and for the quiet window after it ends.
func TestDemandGateBackgroundWaitsForForeground(t *testing.T) {
	// Hold a foreground fetch open on a gate with a short quiet window.
	g := &demandGate{quiet: 50 * time.Millisecond, slots: backgroundFetchSlots}
	leaveForeground := g.enterForeground()

	// A background fetch cannot enter while the foreground fetch runs.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := g.enter(ctx, true, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("background entered beside a foreground fetch: err=%v", err)
	}

	// After the foreground fetch ends, the background fetch enters once the quiet window passes.
	leaveForeground()
	ended := time.Now()
	leave, err := g.enter(t.Context(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	if waited := time.Since(ended); waited < g.quiet {
		t.Fatalf("background entered %v after foreground, want at least %v", waited, g.quiet)
	}
}

// TestDemandGateBackgroundSlots verifies background fetches beyond the slots
// wait for a running one to end.
func TestDemandGateBackgroundSlots(t *testing.T) {
	// Fill the only background slot.
	g := &demandGate{slots: 1}
	leaveFirst, err := g.enter(t.Context(), true, nil)
	if err != nil {
		t.Fatal(err)
	}

	// A second background fetch waits for the slot.
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := g.enter(ctx, true, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second background fetch entered a full gate: err=%v", err)
	}

	// Ending the first fetch admits the next.
	leaveFirst()
	leaveSecond, err := g.enter(t.Context(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	leaveSecond()
}

// TestDemandGatePromotion verifies a promoted background fetch enters at once
// beside a running foreground fetch.
func TestDemandGatePromotion(t *testing.T) {
	// Hold a foreground fetch open and queue a promotable background fetch.
	g := &demandGate{quiet: time.Hour, slots: backgroundFetchSlots}
	leaveForeground := g.enterForeground()
	defer leaveForeground()
	promote := make(chan struct{})
	entered := make(chan error, 1)
	go func() {
		leave, err := g.enter(t.Context(), true, promote)
		if err == nil {
			leave()
		}
		entered <- err
	}()

	// Promotion admits the background fetch without waiting for the
	// foreground fetch.
	close(promote)
	if err := <-entered; err != nil {
		t.Fatal(err)
	}
}

// TestPackReaderForegroundJoinPromotesBackgroundFetch verifies a foreground
// read that joins a background fetch waiting behind foreground work starts
// that fetch at once.
func TestPackReaderForegroundJoinPromotesBackgroundFetch(t *testing.T) {
	// Build a reader whose first transport call stays open until the test ends.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	release := make(chan struct{})
	var held atomic.Bool
	transport := &bytesTransport{data: make([]byte, 1<<20), blockFn: func() {
		if held.CompareAndSwap(false, true) {
			<-release
		}
	}}
	reader := NewPackReader("pack", 1<<20, transport)
	t.Cleanup(reader.Close)
	reader.setTransportWindows(4096, 4096, 4096)

	// Signal each fetch registration.
	started := make(chan struct{}, 8)
	reader.SetStatsChangedCallback(func() {
		select {
		case started <- struct{}{}:
		default:
		}
	})

	// Start a foreground read that holds the link open.
	firstDone := make(chan error, 1)
	go func() {
		_, err := reader.ReaderAt(ctx).ReadAt(make([]byte, 16), 0)
		firstDone <- err
	}()
	waitCallCount(ctx, t, transport, 1)

	// A background read far away registers its fetch and waits at the gate.
	backgroundDone := make(chan error, 1)
	go func() {
		_, err := reader.ReaderAt(block.WithBackground(ctx)).ReadAt(make([]byte, 16), 512<<10)
		backgroundDone <- err
	}()
	for loading := 0; loading < 2; {
		select {
		case <-ctx.Done():
			t.Fatal("background fetch never registered")
		case <-started:
		}
		reader.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			loading = len(reader.loading)
		})
	}
	if got := transport.callCount(); got != 1 {
		t.Fatalf("background fetch reached the transport beside a foreground fetch: %d calls", got)
	}

	// A foreground read of the same range joins and promotes it.
	joinDone := make(chan error, 1)
	go func() {
		_, err := reader.ReaderAt(ctx).ReadAt(make([]byte, 16), 512<<10)
		joinDone <- err
	}()
	for _, done := range []chan error{joinDone, backgroundDone} {
		select {
		case <-ctx.Done():
			t.Fatal("promoted fetch did not run while the foreground fetch was open")
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// Release the held foreground fetch.
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

// waitCallCount waits until transport has seen at least want calls.
func waitCallCount(ctx context.Context, t *testing.T, transport *bytesTransport, want int) {
	t.Helper()
	for {
		ready, wait := transport.callCountAtLeast(want)
		if ready {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("transport never reached %d calls", want)
		case <-wait:
		}
	}
}
