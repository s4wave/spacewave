package plugin_host_web

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// silentWriter accepts every packet and never answers, like a web runtime
// holding a stream for a worker that is gone.
type silentWriter struct{}

// WritePacket drops the packet.
func (silentWriter) WritePacket(*srpc.Packet) error { return nil }

// Close does nothing.
func (silentWriter) Close() error { return nil }

// TestWorkerStreamsFailCallsWhenWorkerGone parks an activation check on a
// worker that never answers, ends the worker, and requires the call to fail
// at once; after the execution closes, a new call fails without opening.
func TestWorkerStreamsFailCallsWhenWorkerGone(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	opened := make(chan struct{}, 4)
	streams := newWorkerStreams(func(context.Context, srpc.PacketDataHandler, srpc.CloseHandler) (srpc.PacketWriter, error) {
		opened <- struct{}{}
		return silentWriter{}, nil
	})
	client := plugin.NewSRPCActivationClient(srpc.NewClient(streams.OpenStream))

	// Park a call, then end the worker's streams.
	done := make(chan error, 1)
	go func() {
		_, err := client.Check(ctx, &plugin.CheckActivationRequest{})
		done <- err
	}()
	<-opened
	select {
	case err := <-done:
		t.Fatalf("call returned before the worker ended: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	streams.endStreams(plugin.ErrWorkerGone)
	select {
	case err := <-done:
		if !plugin.IsWorkerGone(err) {
			t.Fatalf("call failed with %v, want %v", err, plugin.ErrWorkerGone)
		}
	case <-ctx.Done():
		t.Fatal("call still waits after the worker ended")
	}

	// After the execution ends, a call fails without opening a stream.
	streams.close(plugin.ErrWorkerGone)
	if _, err := client.Check(ctx, &plugin.CheckActivationRequest{}); err == nil {
		t.Fatal("call after close succeeded")
	}
	if len(opened) != 0 {
		t.Fatal("call after close opened a stream")
	}
}
