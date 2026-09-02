//go:build js && bldr_cloudflare

package plugin_entrypoint

import (
	"context"
	"errors"
	"syscall/js"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
)

func cfTestFn(t *testing.T, fn func(this js.Value, args []js.Value) any) js.Value {
	t.Helper()
	f := js.FuncOf(fn)
	t.Cleanup(f.Release)
	return f.Value
}

func waitForCloudflareSignal(t *testing.T, ch chan struct{}, label string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %v", label)
	}
}

// TestCloudflarePluginIoValidatesArguments checks that the IO rejects
// non-function globals rather than panicking on untrusted host input.
func TestCloudflarePluginIoValidatesArguments(t *testing.T) {
	okFn := cfTestFn(t, func(this js.Value, args []js.Value) any { return nil })

	if _, err := NewCloudflarePluginIo(js.Undefined(), okFn); err == nil {
		t.Fatal("expected error for undefined open stream func")
	}
	if _, err := NewCloudflarePluginIo(okFn, js.Undefined()); err == nil {
		t.Fatal("expected error for undefined set accept stream func")
	}
	if _, err := NewCloudflarePluginIo(js.ValueOf("nope"), okFn); err == nil {
		t.Fatal("expected error for non-function open stream func")
	}
}

// TestCloudflarePluginIoOpenStreamRejectsMissingGlobal checks that OpenStream
// reports an error when the host global is missing at call time.
func TestCloudflarePluginIoOpenStreamRejectsMissingGlobal(t *testing.T) {
	io, err := NewCloudflarePluginIo(
		cfTestFn(t, func(this js.Value, args []js.Value) any { return nil }),
		cfTestFn(t, func(this js.Value, args []js.Value) any { return nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	io.openStreamFunc = js.Undefined()
	closeErrCh := make(chan error, 1)
	_, err = io.OpenStream(
		context.Background(),
		func(data []byte) error { return nil },
		func(closeErr error) { closeErrCh <- closeErr },
	)
	// NewPushableOpenStream defers the JS invocation to a goroutine; the
	// error surfaces asynchronously through the close handler.
	select {
	case err := <-closeErrCh:
		if err == nil {
			t.Fatal("expected close handler to receive an error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("expected close handler to be invoked for missing global")
	}
}

// TestCloudflarePluginIoSetAcceptStreamsLifecycle checks the full accept
// stream lifecycle: registration, invalid input tolerance, and cleanup on
// context cancellation.
func TestCloudflarePluginIoSetAcceptStreamsLifecycle(t *testing.T) {
	functionCtor := js.Global().Get("Function")
	if functionCtor.IsUndefined() || functionCtor.IsNull() {
		t.Skip("JavaScript Function constructor unavailable")
	}

	callbackSet := make(chan struct{}, 1)
	cleared := make(chan struct{}, 1)
	acceptSetter := js.FuncOf(func(this js.Value, args []js.Value) any {
		if len(args) > 0 && !args[0].IsUndefined() {
			callbackSet <- struct{}{}
		} else {
			cleared <- struct{}{}
		}
		return nil
	})

	io, err := NewCloudflarePluginIo(
		cfTestFn(t, func(this js.Value, args []js.Value) any { return nil }),
		acceptSetter.Value,
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		io.SetAcceptStreams(ctx, srpc.NewMux())
		close(done)
	}()
	waitForCloudflareSignal(t, callbackSet, "accept stream callback registration")

	cancel()
	// Wait until the transport has re-invoked the setter with undefined and
	// the transport goroutine has fully returned, so no JS callback outlives
	// the test.
	select {
	case <-cleared:
	case <-time.After(5 * time.Second):
		t.Fatal("accept stream registration was not cleared after cancel")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SetAcceptStreams did not return after context cancel")
	}
	// Yield to the JS event loop so the setter Invoke fully unwinds before
	// the test binary exits.
	yieldDone := make(chan struct{})
	setTimeout := js.Global().Get("setTimeout")
	setTimeout.Invoke(js.FuncOf(func(this js.Value, args []js.Value) any {
		yieldDone <- struct{}{}
		return nil
	}), 0)
	select {
	case <-yieldDone:
	case <-time.After(5 * time.Second):
		t.Fatal("event loop yield timed out")
	}
}

var errDeferredFlush = errors.New("deferred flush failed")

type failingPacketWriter struct {
	packet *srpc.Packet
}

func (w *failingPacketWriter) WritePacket(packet *srpc.Packet) error {
	w.packet = packet
	return errDeferredFlush
}

func (w *failingPacketWriter) Close() error { return nil }

var _ srpc.PacketWriter = (*failingPacketWriter)(nil)

func TestDeferredPacketWriterOwnsPacketsAndRetainsFlushError(t *testing.T) {
	writer := newDeferredPacketWriter()
	packet := srpc.NewCallStartPacket("service", "method", []byte("original"), false)
	if err := writer.WritePacket(packet); err != nil {
		t.Fatal(err)
	}
	packet.GetCallStart().Data[0] = 'X'

	resolved := &failingPacketWriter{}
	writer.setWriter(resolved)
	if got := string(resolved.packet.GetCallStart().Data); got != "original" {
		t.Fatalf("flushed packet data = %q, want owned clone", got)
	}
	if err := writer.WritePacket(srpc.NewCallCancelPacket()); !errors.Is(err, errDeferredFlush) {
		t.Fatalf("WritePacket error = %v, want retained flush error", err)
	}
	if err := writer.Close(); !errors.Is(err, errDeferredFlush) {
		t.Fatalf("Close error = %v, want retained flush error", err)
	}
}
