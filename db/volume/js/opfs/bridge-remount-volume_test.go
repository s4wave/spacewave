//go:build js

package volume_opfs

import (
	"context"
	"errors"
	"testing"
	"time"
)

// newTestRemountVolume builds a wrapper with injected store and swap funcs and
// no underlying Opfs, so the remount lifecycle runs without a browser bridge.
func newTestRemountVolume(execStore, waitSwap func(context.Context) error) *bridgeRemountVolume {
	return &bridgeRemountVolume{executeStore: execStore, waitSwap: waitSwap}
}

// TestBridgeRemountVolumeKeepsMountedAfterStoreNoop proves a nil store Execute
// return (the OPFS store has no background task) does not end the volume; Execute
// keeps waiting and ends with a transient remount error only once the bridge
// swaps.
func TestBridgeRemountVolumeKeepsMountedAfterStoreNoop(t *testing.T) {
	swapCh := make(chan struct{})
	v := newTestRemountVolume(
		func(context.Context) error { return nil },
		func(ctx context.Context) error {
			select {
			case <-swapCh:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- v.Execute(ctx) }()

	select {
	case err := <-done:
		t.Fatalf("Execute returned before swap: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(swapCh)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Execute must return a remount error on swap")
		}
		if errors.Is(err, context.Canceled) {
			t.Fatalf("swap remount must not be a context error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute did not return after swap")
	}
}

// TestBridgeRemountVolumeReturnsStoreError proves a real store error ends Execute
// so the controller remounts.
func TestBridgeRemountVolumeReturnsStoreError(t *testing.T) {
	wantErr := errors.New("store boom")
	v := newTestRemountVolume(
		func(context.Context) error { return wantErr },
		func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	)
	if err := v.Execute(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("Execute = %v, want %v", err, wantErr)
	}
}

// TestBridgeRemountVolumeContextCancel proves Execute returns the context error
// when canceled with no swap and a long-running store.
func TestBridgeRemountVolumeContextCancel(t *testing.T) {
	v := newTestRemountVolume(
		func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
		func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- v.Execute(ctx) }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute did not return after cancel")
	}
}
