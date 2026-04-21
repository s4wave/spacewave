package cstate

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestCState tests the CState type.
func TestCState(t *testing.T) {
	ctx := context.Background()
	st := NewCState(0)
	var lastState atomic.Int32
	go func() {
		_ = st.Execute(ctx, nil)
	}()
	_, _ = st.AddWatcher(ctx, true, func(ctx context.Context, state int) {
		lastState.Store(int32(state)) //nolint:gosec
	})
	_, err := st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		v.SetObj(1)
		return true, nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if lsl := lastState.Load(); lsl != 1 {
		t.Fatal("Expected lastState to be 1, got", lsl)
	}
}

// TestCStateMultipleOperations tests multiple operations on CState.
func TestCStateMultipleOperations(t *testing.T) {
	ctx := context.Background()
	st := NewCState(0)
	go func() {
		_ = st.Execute(ctx, nil)
	}()

	for i := 1; i <= 5; i++ {
		_, err := st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
			v.SetObj(i)
			return true, nil
		})
		if err != nil {
			t.Fatalf("Error applying operation %d: %v", i, err)
		}
	}

	err := st.Wait(ctx, func(ctx context.Context, val int) (bool, error) {
		return val == 5, nil
	})
	if err != nil {
		t.Fatal("Error waiting for final state:", err)
	}

	var finalState int
	err = st.View(ctx, func(ctx context.Context, value int) error {
		finalState = value
		return nil
	})
	if err != nil {
		t.Fatal("Error viewing final state:", err)
	}
	if finalState != 5 {
		t.Fatalf("Expected final state to be 5, got %d", finalState)
	}
}

// TestCStateWatchers tests adding and removing watchers.
func TestCStateWatchers(t *testing.T) {
	ctx := context.Background()
	st := NewCState(0)
	go func() {
		_ = st.Execute(ctx, nil)
	}()

	watcherCalls := make(map[int]int)
	var mu sync.Mutex

	remove1, err := st.AddWatcher(ctx, true, func(ctx context.Context, state int) {
		mu.Lock()
		watcherCalls[1]++
		mu.Unlock()
	})
	if err != nil {
		t.Fatal("Error adding watcher 1:", err)
	}

	remove2, err := st.AddWatcher(ctx, false, func(ctx context.Context, state int) {
		mu.Lock()
		watcherCalls[2]++
		mu.Unlock()
	})
	if err != nil {
		t.Fatal("Error adding watcher 2:", err)
	}

	_, err = st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		v.SetObj(1)
		return true, nil
	})
	if err != nil {
		t.Fatal("Error applying state change:", err)
	}

	remove1()
	remove2()

	_, err = st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		v.SetObj(2)
		return true, nil
	})
	if err != nil {
		t.Fatal("Error applying second state change:", err)
	}

	time.Sleep(100 * time.Millisecond) // Give some time for potential watcher calls

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
	ctx := context.Background()
	st := NewCState(0)
	go func() {
		_ = st.Execute(ctx, nil)
	}()

	expectedError := errors.New("test error")
	_, err := st.Apply(ctx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		return false, expectedError
	})
	if err != expectedError {
		t.Fatalf("Expected error %v, got %v", expectedError, err)
	}

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()

	_, err = st.Apply(canceledCtx, func(ctx context.Context, v *CStateWriter[int]) (dirty bool, err error) {
		return true, nil
	})
	if err != context.Canceled {
		t.Fatalf("Expected context.Canceled error, got %v", err)
	}
}
