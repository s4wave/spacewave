package provider_spacewave

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
)

// runConcurrentSeed launches one owner goroutine, waits until it has
// entered fetchFn (i.e. providerSeed.inflight=true), then launches
// (callers-1) waiter goroutines and lets them park on the broadcast
// before unblocking the owner. Returns once every caller has returned
// from Run. Errors per caller are returned in launch order.
func runConcurrentSeed(t *testing.T, callers int, fetchErr error) ([]error, int32) {
	t.Helper()
	var (
		seed   providerSeed
		bcast  broadcast.Broadcast
		fetchN atomic.Int32
	)
	gate := make(chan struct{})
	ownerStarted := make(chan struct{})

	fetchFn := func(ctx context.Context) error {
		fetchN.Add(1)
		close(ownerStarted)
		<-gate
		return fetchErr
	}

	errs := make([]error, callers)
	var wg sync.WaitGroup
	wg.Go(func() {
		errs[0] = seed.Run(t.Context(), &bcast, fetchFn)
	})

	select {
	case <-ownerStarted:
	case <-time.After(time.Second):
		t.Fatal("owner did not enter fetchFn")
	}

	wg.Add(callers - 1)
	for i := 1; i < callers; i++ {
		go func(idx int) {
			defer wg.Done()
			errs[idx] = seed.Run(t.Context(), &bcast, fetchFn)
		}(i)
	}

	// Give the waiters a tick to park on the broadcast wait channel
	// before letting the owner finish.
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()

	return errs, fetchN.Load()
}

// TestProviderSeedSingleflight asserts that N concurrent Run callers
// share one fetchFn invocation and observe the same nil result.
func TestProviderSeedSingleflight(t *testing.T) {
	errs, fetchN := runConcurrentSeed(t, 16, nil)
	if fetchN != 1 {
		t.Fatalf("fetchFn called %d times, want 1", fetchN)
	}
	for i, err := range errs {
		if err != nil {
			t.Fatalf("caller %d returned %v, want nil", i, err)
		}
	}
}

// TestProviderSeedSharesError asserts that all waiters observe the
// owner's error result.
func TestProviderSeedSharesError(t *testing.T) {
	wantErr := errors.New("seed failure")
	errs, fetchN := runConcurrentSeed(t, 8, wantErr)
	if fetchN != 1 {
		t.Fatalf("fetchFn called %d times, want 1", fetchN)
	}
	for i, err := range errs {
		if !errors.Is(err, wantErr) {
			t.Fatalf("caller %d returned %v, want %v", i, err, wantErr)
		}
	}
}

// TestProviderSeedSecondPassRefetches asserts that after a completed Run
// the next call fires fetchFn again (singleflight, not memoization).
func TestProviderSeedSecondPassRefetches(t *testing.T) {
	var (
		seed   providerSeed
		bcast  broadcast.Broadcast
		fetchN atomic.Int32
	)
	fetchFn := func(ctx context.Context) error {
		fetchN.Add(1)
		return nil
	}

	for range 3 {
		if err := seed.Run(t.Context(), &bcast, fetchFn); err != nil {
			t.Fatalf("Run: %v", err)
		}
	}
	if got := fetchN.Load(); got != 3 {
		t.Fatalf("fetchFn called %d times, want 3", got)
	}
}

// TestProviderSeedWaiterContextCanceled asserts that a waiter parked on
// the broadcast returns ctx.Err() when its context is canceled, without
// affecting the owner.
func TestProviderSeedWaiterContextCanceled(t *testing.T) {
	var (
		seed   providerSeed
		bcast  broadcast.Broadcast
		fetchN atomic.Int32
	)
	gate := make(chan struct{})
	ownerStarted := make(chan struct{})

	fetchFn := func(ctx context.Context) error {
		fetchN.Add(1)
		close(ownerStarted)
		<-gate
		return nil
	}

	ownerDone := make(chan error, 1)
	go func() { ownerDone <- seed.Run(t.Context(), &bcast, fetchFn) }()

	select {
	case <-ownerStarted:
	case <-time.After(time.Second):
		t.Fatal("owner did not enter fetchFn")
	}

	waiterCtx, waiterCancel := context.WithCancel(t.Context())
	waiterDone := make(chan error, 1)
	go func() { waiterDone <- seed.Run(waiterCtx, &bcast, fetchFn) }()

	// Let the waiter park inside Run's select on waitCh.
	time.Sleep(20 * time.Millisecond)
	waiterCancel()

	select {
	case err := <-waiterDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waiter returned %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not return after cancel")
	}

	close(gate)
	if err := <-ownerDone; err != nil {
		t.Fatalf("owner returned %v, want nil", err)
	}
	if got := fetchN.Load(); got != 1 {
		t.Fatalf("fetchFn called %d times, want 1", got)
	}
}
