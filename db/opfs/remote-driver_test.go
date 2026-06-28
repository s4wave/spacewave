//go:build js

package opfs

import (
	"context"
	"syscall/js"
	"testing"
	"time"
)

// makeBridgePort builds a minimal object that passes remotePortUsable: an object
// exposing a request member. The function body is never called by these tests.
func makeBridgePort(t *testing.T) js.Value {
	t.Helper()
	port := js.Global().Get("Object").New()
	fn := js.FuncOf(func(js.Value, []js.Value) any { return js.Undefined() })
	t.Cleanup(fn.Release)
	port.Set("request", fn)
	return port
}

// TestRemoteDriverInstallPortSwapGeneration proves only a genuine port swap
// advances the swap generation and drops the stale handle id space; the first
// install and a same-port reinstall do not.
func TestRemoteDriverInstallPortSwapGeneration(t *testing.T) {
	p1 := makeBridgePort(t)
	d := NewRemoteDriver(p1)
	if d.swapGen != 0 {
		t.Fatalf("first install must not be a swap: swapGen=%d", d.swapGen)
	}

	d.installPort(p1)
	if d.swapGen != 0 {
		t.Fatalf("same-port reinstall must not swap: swapGen=%d", d.swapGen)
	}

	if _, err := d.newHandle(js.ValueOf(7), remoteHandleKindDirectory); err != nil {
		t.Fatalf("seed handle: %v", err)
	}
	d.installPort(makeBridgePort(t))
	if d.swapGen != 1 {
		t.Fatalf("distinct-port install must swap: swapGen=%d", d.swapGen)
	}
	if len(d.handles) != 0 {
		t.Fatalf("swap must clear stale handles, got %d", len(d.handles))
	}
}

// TestRemoteDriverWaitSwapWakesOnSwap proves a waiter parked before a swap wakes
// once the bridge port is replaced.
func TestRemoteDriverWaitSwapWakesOnSwap(t *testing.T) {
	d := NewRemoteDriver(makeBridgePort(t))
	result := make(chan error, 1)
	go func() { result <- d.WaitSwap(context.Background()) }()

	ports := []js.Value{makeBridgePort(t), makeBridgePort(t)}
	for i := 0; i < 200; i++ {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("WaitSwap returned error: %v", err)
			}
			return
		case <-time.After(5 * time.Millisecond):
			d.installPort(ports[i%2])
		}
	}
	t.Fatal("WaitSwap did not wake after bridge swaps")
}

// TestRemoteDriverWaitSwapContextCanceled proves WaitSwap returns the context
// error when no swap occurs and the context is canceled.
func TestRemoteDriverWaitSwapContextCanceled(t *testing.T) {
	d := NewRemoteDriver(makeBridgePort(t))
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- d.WaitSwap(ctx) }()

	cancel()
	if err := <-result; err == nil {
		t.Fatal("WaitSwap must return the context error when canceled")
	}
}
