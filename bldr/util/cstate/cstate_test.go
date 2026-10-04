package cstate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// TestCState tests the CState type.
func TestCState(t *testing.T) {
	// Start CState and prepare an atomic record of watcher updates.
	ctx := context.Background()
	st := NewCState(0)
	var lastState atomic.Int32
	go func() {
		_ = st.Execute(ctx, nil)
	}()

	// Register a watcher that records the current integer state.
	_, _ = st.AddWatcher(ctx, true, func(ctx context.Context, state int) {
		lastState.Store(int32(state)) //nolint:gosec
	})

	// Apply one state update through the CState operation queue.
	_, err := st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		v.SetObj(1)
		return true, nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert that the watcher observed the applied state.
	if lsl := lastState.Load(); lsl != 1 {
		t.Fatal("Expected lastState to be 1, got", lsl)
	}
}

// TestCStateMultipleOperations tests multiple operations on CState.
func TestCStateMultipleOperations(t *testing.T) {
	// Start CState before queuing multiple state updates.
	ctx := context.Background()
	st := NewCState(0)
	go func() {
		_ = st.Execute(ctx, nil)
	}()

	// Apply five updates and report the first rejected operation.
	for i := 1; i <= 5; i++ {
		_, err := st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
			v.SetObj(i)
			return true, nil
		})
		if err != nil {
			t.Fatalf("Error applying operation %d: %v", i, err)
		}
	}

	// Wait until the CState watcher observes the final value.
	err := st.Wait(ctx, func(ctx context.Context, val int) (bool, error) {
		return val == 5, nil
	})
	if err != nil {
		t.Fatal("Error waiting for final state:", err)
	}

	// Read the completed state while CState holds its view lock.
	var finalState int
	err = st.View(ctx, func(ctx context.Context, value int) error {
		finalState = value
		return nil
	})
	if err != nil {
		t.Fatal("Error viewing final state:", err)
	}

	// Assert the view returns the last queued value.
	if finalState != 5 {
		t.Fatalf("Expected final state to be 5, got %d", finalState)
	}
}

// TestCStateWatchers tests adding and removing watchers.
func TestCStateWatchers(t *testing.T) {
	// Start CState before registering its test watchers.
	ctx := context.Background()
	st := NewCState(0)
	go func() {
		_ = st.Execute(ctx, nil)
	}()

	// Count callbacks under a mutex shared by both watchers.
	watcherCalls := make(map[int]int)
	var mu sync.Mutex

	// Register the initial watcher and check registration succeeds.
	remove1, err := st.AddWatcher(ctx, true, func(ctx context.Context, state int) {
		mu.Lock()
		watcherCalls[1]++
		mu.Unlock()
	})
	if err != nil {
		t.Fatal("Error adding watcher 1:", err)
	}

	// Register the second watcher without an initial delivery.
	remove2, err := st.AddWatcher(ctx, false, func(ctx context.Context, state int) {
		mu.Lock()
		watcherCalls[2]++
		mu.Unlock()
	})
	if err != nil {
		t.Fatal("Error adding watcher 2:", err)
	}

	// Apply the first state change to invoke both registered watchers.
	_, err = st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		v.SetObj(1)
		return true, nil
	})
	if err != nil {
		t.Fatal("Error applying state change:", err)
	}

	// Remove both watchers before sending another state change.
	remove1()
	remove2()

	// Apply a second state change after both watchers are removed.
	_, err = st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		v.SetObj(2)
		return true, nil
	})
	if err != nil {
		t.Fatal("Error applying second state change:", err)
	}

	// Execute holds the lock while it calls watchers, so taking it here waits
	// for any watcher calls from the second change.
	err = st.View(ctx, func(ctx context.Context, value int) error {
		return nil
	})
	if err != nil {
		t.Fatal("Error waiting for watcher calls:", err)
	}

	// Verify removal stops later calls without changing earlier counts.
	mu.Lock()
	if watcherCalls[1] != 2 {
		t.Fatalf("Expected watcher 1 to be called 2 times, got %d", watcherCalls[1])
	}
	if watcherCalls[2] != 1 {
		t.Fatalf("Expected watcher 2 to be called 1 time, got %d", watcherCalls[2])
	}
	mu.Unlock()
}

// TestCStateErrorHandling tests error handling in CState operations.
func TestCStateErrorHandling(t *testing.T) {
	// Start CState before checking operation error propagation.
	ctx := context.Background()
	st := NewCState(0)
	go func() {
		_ = st.Execute(ctx, nil)
	}()

	// Queue an operation failure and verify CState returns that error.
	expectedError := errors.New("test error")
	_, err := st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		return false, expectedError
	})
	if err != expectedError {
		t.Fatalf("Expected error %v, got %v", expectedError, err)
	}

	// Prepare a canceled context for the next queued operation.
	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	// Verify CState rejects work submitted after context cancellation.
	_, err = st.Apply(canceledCtx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		return true, nil
	})
	if err != context.Canceled {
		t.Fatalf("Expected context.Canceled error, got %v", err)
	}
}
